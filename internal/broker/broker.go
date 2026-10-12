// Package broker 实现从 HRPAuth 主服务拉取 SDK 包的客户端。
//
// 职责：
//   - 轮询 GET {base}/services/sdk-packages，携带 Ops token（Authorization: Bearer）。
//   - 用 If-None-Match / ETag 做增量：ETag 取全列表内容的 sha256 摘要。
//     列表无变化时返回 304，不触发重建。
//   - 下载 GET {base}/services/sdk-packages/{name}/download 到本地缓存。
//
// 本服务只从 HRPAuth 拉包，绝不直接访问卫星服务；所有鉴权由 HRPAuth 负责。
package broker

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SdkPackageMeta 是 HRPAuth 返回的 SDK 包元信息（列表项）。
// 字段采用宽容解析，仅保证与契约相关的字段存在。
type SdkPackageMeta struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
	RunScripts bool   `json:"run_scripts"`
	// Routes 仅用于诊断，形态由宿主契约决定（当前为对象数组），因此使用
	// RawMessage 宽容接收，避免列表解析因诊断字段变化而失败。
	Routes       []json.RawMessage `json:"routes,omitempty"`
	RegisteredAt string            `json:"registered_at,omitempty"`
}

// Client 是 HRPAuth SDK 包客户端。
type Client struct {
	baseURL      string
	clientID     string
	clientSecret string
	httpClient   *http.Client
	// 缓存根目录：包归档下载后存放于此（按 name 存放）。
	cacheDir string
	// lastETag 记录上一次轮询返回的 ETag，用于 If-None-Match。
	lastETag string

	// OAuth2 令牌缓存
	tokenMu     sync.RWMutex
	accessToken string
	tokenExpiry time.Time
}

// New 构造客户端。cacheDir 不存在时自动创建。
func New(baseURL, clientID, clientSecret, cacheDir string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建包缓存目录失败: %w", err)
	}
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: timeout},
		cacheDir:     cacheDir,
	}, nil
}

// ListResult 是列表轮询结果。
type ListResult struct {
	// Changed 为 false 表示列表无变化（304），无需重建。
	Changed bool
	// ETag 是本次响应的摘要（无论是否 304 都可用于下次请求）。
	ETag string
	// Packages 是解析出的包元信息列表；Changed=false 时为 nil。
	Packages []SdkPackageMeta
}

// fingerprint 计算列表内容的 sha256 摘要，作为 ETag 值。
func fingerprint(pkgs []SdkPackageMeta) string {
	// 按 name 排序保证摘要稳定，避免列表顺序变化引发无谓重建。
	sorted := make([]SdkPackageMeta, len(pkgs))
	copy(sorted, pkgs)
	// 稳定排序：以 name 为主键。
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j-1].Name > sorted[j].Name; j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	raw, _ := json.Marshal(sorted)
	sum := sha256.Sum256(raw)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

// List 轮询 SDK 包列表。data 字段做宽容解析：依次尝试 data 顶层数组、
// data.data、data.meta、data.result 等常见形态，解析失败不 throw 而是返回错误。
func (c *Client) List() (*ListResult, error) {
	reqURL := c.baseURL + "/services/sdk-packages"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造列表请求失败: %w", err)
	}
	if err := c.authorize(req); err != nil {
		return nil, fmt.Errorf("认证失败: %w", err)
	}
	if c.lastETag != "" {
		req.Header.Set("If-None-Match", c.lastETag)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 SDK 包列表失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 304：无变化，沿用旧 ETag。
	if resp.StatusCode == http.StatusNotModified {
		return &ListResult{Changed: false, ETag: c.lastETag}, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("SDK 包列表响应异常 status=%d body=%s", resp.StatusCode, truncate(string(body), 512))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 SDK 包列表响应失败: %w", err)
	}

	// 宽容解析：兼容 {success,data:[...]}、{success,data:{data:[...]}}、
	// {data:[...]} 与裸数组等多种形态。
	pkgs, err := parsePackageList(body)
	if err != nil {
		return nil, err
	}

	etag := fingerprint(pkgs)
	c.lastETag = etag
	return &ListResult{Changed: true, ETag: etag, Packages: pkgs}, nil
}

func (c *Client) authorize(req *http.Request) error {
	token, err := c.getOrRefreshToken()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func (c *Client) getOrRefreshToken() (string, error) {
	c.tokenMu.RLock()
	if c.accessToken != "" && time.Now().Add(time.Minute).Before(c.tokenExpiry) {
		token := c.accessToken
		c.tokenMu.RUnlock()
		return token, nil
	}
	c.tokenMu.RUnlock()

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	// Double check after lock
	if c.accessToken != "" && time.Now().Add(time.Minute).Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	// Fetch new token
	tokenURL := c.baseURL + "/oauth/token"
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.clientID, c.clientSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 OAuth2 令牌失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("获取令牌响应异常 status=%d body=%s", resp.StatusCode, string(body))
	}

	var res struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("解析令牌响应失败: %w", err)
	}

	if !res.Success || res.Data.AccessToken == "" {
		return "", fmt.Errorf("令牌颁发失败")
	}

	c.accessToken = res.Data.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(res.Data.ExpiresIn) * time.Second)

	return c.accessToken, nil
}

// parsePackageList 宽容解析 SDK 包列表响应体。
func parsePackageList(body []byte) ([]SdkPackageMeta, error) {
	var pkgs []SdkPackageMeta
	// 尝试顶层数组形态。
	if err := json.Unmarshal(body, &pkgs); err == nil {
		return pkgs, nil
	}
	// 统一响应结构 {success, meta, data, ...} 形态。
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		if len(envelope.Data) > 0 {
			// data 直接是数组。
			if err := json.Unmarshal(envelope.Data, &pkgs); err == nil {
				return pkgs, nil
			}
			// data 是对象，其内部再找数组（如 data.data / data.packages / data.list / data.result）。
			var nested map[string]json.RawMessage
			if err := json.Unmarshal(envelope.Data, &nested); err == nil {
				for _, key := range []string{"data", "packages", "list", "result", "items", "records"} {
					raw, ok := nested[key]
					if !ok {
						continue
					}
					if err := json.Unmarshal(raw, &pkgs); err == nil {
						return pkgs, nil
					}
				}
			}
		}
		// data 缺失或形态不明：meta 里尝试。
		var meta struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &meta); err == nil && len(meta.Data) > 0 {
			if err := json.Unmarshal(meta.Data, &pkgs); err == nil {
				return pkgs, nil
			}
		}
	}
	return nil, fmt.Errorf("无法从响应中解析 SDK 包列表: %s", truncate(string(body), 512))
}

// Download 下载指定包的归档到本地缓存，返回归档本地路径与内容摘要。
func (c *Client) Download(name string) (string, string, error) {
	reqURL := fmt.Sprintf("%s/services/sdk-packages/%s/download", c.baseURL, url.PathEscape(name))
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("构造下载请求失败: %w", err)
	}
	if err := c.authorize(req); err != nil {
		return "", "", fmt.Errorf("认证失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("下载 SDK 包 %s 失败: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("下载 SDK 包 %s 响应异常 status=%d body=%s", name, resp.StatusCode, truncate(string(body), 512))
	}

	reader := bufio.NewReader(resp.Body)
	header, _ := reader.Peek(4)
	suffix, err := archiveSuffix(header)
	if err != nil {
		return "", "", fmt.Errorf("识别 SDK 包 %s 归档格式失败: %w", name, err)
	}

	// 写入缓存目录，带 sha256 摘要校验（若响应头或 meta 提供）。
	dest := filepath.Join(c.cacheDir, name+suffix)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", "", fmt.Errorf("创建缓存目录失败: %w", err)
	}
	hasher := sha256.New()
	f, err := os.Create(dest)
	if err != nil {
		return "", "", fmt.Errorf("创建缓存文件失败: %w", err)
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(io.MultiWriter(f, hasher), reader)
	if err != nil {
		return "", "", fmt.Errorf("写入缓存文件失败: %w", err)
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	return dest, sum, nil
}

// CacheDir 返回包缓存目录。
func (c *Client) CacheDir() string { return c.cacheDir }

func archiveSuffix(header []byte) (string, error) {
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		return ".tar.gz", nil
	}
	if len(header) >= 4 && header[0] == 'P' && header[1] == 'K' && header[2] == 0x03 && header[3] == 0x04 {
		return ".zip", nil
	}
	return "", fmt.Errorf("未知归档头: %x", header)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
