// Package config 负责加载与校验 SDKHandler 的 YAML 配置。
package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// ConfigVersion 是当前代码预期的配置文件版本。
	// 当配置结构发生不兼容变更时需增加此版本号，并在 migrations.go 中编写迁移逻辑。
	ConfigVersion = "4"
)

// Config 是顶层配置结构。
type Config struct {
	Version        string        `yaml:"version"`
	Server         ServerConfig  `yaml:"server"`
	HRPAuth        HRPAuthConfig `yaml:"hrpauth"`
	Build          BuildConfig   `yaml:"build"`
	StateDir       string        `yaml:"state_dir"`
	ServeArtifacts bool          `yaml:"serve_artifacts"`
	configPath     string        // 配置文件绝对路径（用于解析相对路径）
}

// ServerConfig 是 HTTP 服务配置。
type ServerConfig struct {
	ListenAddr      string `yaml:"listen_addr"`
	MaintenanceAddr string `yaml:"maintenance_addr"`
}

// HRPAuthConfig 是 HRPAuth 主服务连接配置。
type HRPAuthConfig struct {
	BaseURL             string `yaml:"base_url"`
	ClientID            string `yaml:"client_id"`
	ClientSecret        string `yaml:"client_secret"`
	PollIntervalSeconds int    `yaml:"poll_interval_seconds"`
}

// BuildConfig 是构建与发布管线配置。
type BuildConfig struct {
	TemplateDir          string   `yaml:"template_dir"`
	TemplateGitURL       string   `yaml:"template_git_url"`
	WorkDir              string   `yaml:"work_dir"`
	ArtifactsDir         string   `yaml:"artifacts_dir"`
	CurrentLink          string   `yaml:"current_link"`
	NPMCommand           string   `yaml:"npm_command"`
	IgnoreScriptsDefault bool     `yaml:"ignore_scripts_default"`
	NPMBlacklist         []string `yaml:"npm_blacklist"`
	BuildTimeout         int      `yaml:"build_timeout_seconds"`          // 单次构建整体超时（秒），0 表示默认 20 分钟
	TemplatePollInterval int      `yaml:"template_poll_interval_seconds"` // 自动拉取模板的时间间隔（秒），0 表示不自动拉取
}

// Load 读取并解析配置文件。
// 如果文件不存在，则创建一个带有默认值的示例文件。
// 如果版本不匹配，则尝试执行自动迁移。
func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := CreateDefault(path); err != nil {
			return nil, fmt.Errorf("创建默认配置文件失败: %w", err)
		}
		return nil, fmt.Errorf("配置文件 %s 不存在，已创建默认配置，请修改后重启", path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var yamlConfig map[string]interface{}
	if err := yaml.Unmarshal(raw, &yamlConfig); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}

	// 检查版本并执行迁移
	version, _ := yamlConfig["version"].(string)
	if version != ConfigVersion {
		fmt.Printf("检测到配置版本 %s，当前程序预期版本 %s，开始执行自动迁移...\n", version, ConfigVersion)
		if err := Migrate(path, yamlConfig); err != nil {
			return nil, fmt.Errorf("配置迁移失败: %w", err)
		}
		// 迁移后重新读取
		raw, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取迁移后的配置文件失败: %w", err)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	cfg.configPath = path

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	// 相对路径统一基于配置文件所在目录解析，避免受启动 cwd 影响。
	cfg.resolveRelativePaths()
	return &cfg, nil
}

// CreateDefault 创建一个带有默认值的配置文件。
func CreateDefault(path string) error {
	cfg := &Config{
		Version: ConfigVersion,
		Server: ServerConfig{
			ListenAddr:      ":2704",
			MaintenanceAddr: ":2705",
		},
		HRPAuth: HRPAuthConfig{
			BaseURL:             "http://127.0.0.1:2778",
			ClientID:            "",
			ClientSecret:        "",
			PollIntervalSeconds: 60,
		},
		Build: BuildConfig{
			TemplateDir:          "./HRPAuth-WebUI",
			TemplateGitURL:       "https://github.com/CoreMatch/HRPAuth-WebUI.git",
			WorkDir:              "./frontend-build",
			ArtifactsDir:         "./dist",
			CurrentLink:          "./dist",
			NPMCommand:           "npm",
			IgnoreScriptsDefault: true,
			TemplatePollInterval: 0, // 默认不自动拉取
		},
		StateDir:       "./state",
		ServeArtifacts: false,
	}

	return cfg.Save(path)
}

// Save 将配置保存到指定路径。
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	// 写入临时文件再重命名，保证原子性
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

// Backup 创建配置文件的备份。
func Backup(path string, version string) error {
	backupPath := fmt.Sprintf("%s.bak.%s", path, version)
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (c *Config) applyDefaults() {
	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = ":2704"
	}
	if c.Server.MaintenanceAddr == "" {
		c.Server.MaintenanceAddr = ":2705"
	}
	if c.HRPAuth.BaseURL == "" {
		c.HRPAuth.BaseURL = "http://127.0.0.1:2778"
	}
	if c.HRPAuth.PollIntervalSeconds <= 0 {
		c.HRPAuth.PollIntervalSeconds = 60
	}
	if c.Build.TemplateDir == "" {
		c.Build.TemplateDir = "../HRPAuth-WebUI"
	}
	if c.Build.TemplateGitURL == "" {
		c.Build.TemplateGitURL = "https://github.com/CoreMatch/HRPAuth-WebUI.git"
	}
	if c.Build.WorkDir == "" {
		c.Build.WorkDir = "./frontend-build"
	}
	if c.Build.ArtifactsDir == "" {
		c.Build.ArtifactsDir = "./dist"
	}
	if c.Build.CurrentLink == "" {
		c.Build.CurrentLink = "./dist"
	}
	if c.Build.NPMCommand == "" {
		c.Build.NPMCommand = "npm"
	}
	if c.Build.BuildTimeout <= 0 {
		c.Build.BuildTimeout = 20 * 60
	}
	if c.StateDir == "" {
		c.StateDir = "./state"
	}
}

func (c *Config) validate() error {
	var missing []string
	if strings.TrimSpace(c.HRPAuth.ClientID) == "" {
		missing = append(missing, "hrpauth.client_id")
	}
	if strings.TrimSpace(c.HRPAuth.ClientSecret) == "" {
		missing = append(missing, "hrpauth.client_secret")
	}
	if strings.TrimSpace(c.Build.TemplateDir) == "" {
		missing = append(missing, "build.template_dir")
	}
	if len(missing) > 0 {
		return fmt.Errorf("配置缺少必填项: %s", strings.Join(missing, ", "))
	}
	if !c.Build.IgnoreScriptsDefault {
		// 允许 operator 显式放开；契约默认 true，此处仅做提示，不强制。
	}
	return nil
}

// resolveRelativePaths 把相对路径基于配置文件所在目录补全为绝对路径。
func (c *Config) resolveRelativePaths() {
	base := dirOf(c.configPath)
	if base == "" {
		return
	}
	c.Build.TemplateDir = absIfRelative(base, c.Build.TemplateDir)
	c.Build.WorkDir = absIfRelative(base, c.Build.WorkDir)
	c.Build.ArtifactsDir = absIfRelative(base, c.Build.ArtifactsDir)
	c.Build.CurrentLink = absIfRelative(base, c.Build.CurrentLink)
	c.StateDir = absIfRelative(base, c.StateDir)
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

func absIfRelative(base, p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return base + "/" + strings.TrimPrefix(p, "./")
}
