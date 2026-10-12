package inject

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Sdk 是注入阶段使用的 SDK 包实体（解包后）。
type Sdk struct {
	// Name 与 Manifest.Name 一致。
	Name string
	// Manifest 是校验通过的包 manifest。
	Manifest *Manifest
	// SHA256 是包归档内容的摘要（来自 HRPAuth 元信息或下载后本地计算）。
	SHA256 string
	// Dir 是解包目录（work_dir/src/generated/services/<name>）。
	Dir string
	// PkgDir 是包内根目录对应生成目录中的相对路径（如 services/foo）。
	PkgDir string
}

// Result 是一次注入的完整结果，供构建管线与诊断使用。
type Result struct {
	// WorkDir 是本次注入后的工作目录。
	WorkDir string
	// SDKList 是参与注入的 SDK 名称列表（排序）。
	SDKList []string
	// ManifestSums 是 name -> sha256 摘要。
	ManifestSums map[string]string
	// SDKVersion 是本次注入的 SDK 集合摘要指纹（用于状态记录）。
	SDKVersion string
	// SDKs 是解包后的 SDK 列表。
	SDKs []*Sdk
	// ValidationResults 记录每个包的校验结果（name -> 错误列表，空表示通过）。
	ValidationResults map[string][]string
	// RunScripts 记录每个包是否允许执行 npm scripts（run_scripts:true）。
	RunScripts map[string]bool
}

// Options 是注入阶段的输入参数。
type Options struct {
	// WorkDir 本次构建工作目录（应已复制模板）。
	WorkDir string
	// PackageArchives 是 name -> 归档文件路径。
	PackageArchives map[string]string
	// PackageHashes 是 name -> 归档内容 sha256（可选，缺失时本地计算）。
	PackageHashes map[string]string
	// IgnoreScriptsDefault 与契约默认一致（仅影响记录，不改变注入行为）。
	IgnoreScriptsDefault bool
	// RunScripts 是每个包是否允许执行 npm scripts 的记录。
	RunScripts map[string]bool
}

// Inject 执行完整注入流程：解包 -> 校验 -> 依赖合并 -> 生成四文件。
func Inject(opts Options) (*Result, error) {
	res := &Result{
		WorkDir:           opts.WorkDir,
		ManifestSums:      map[string]string{},
		ValidationResults: map[string][]string{},
		SDKVersion:        "empty",
	}

	// 1. 解包每个 SDK 到 work_dir/src/generated/services/<name>/。
	if err := os.MkdirAll(filepath.Join(opts.WorkDir, "src", "generated", "services"), 0o755); err != nil {
		return nil, fmt.Errorf("创建 generated/services 目录失败: %w", err)
	}

	names := make([]string, 0, len(opts.PackageArchives))
	for name := range opts.PackageArchives {
		names = append(names, name)
	}
	sort.Strings(names)

	var allDeps = map[string]string{}
	runScriptsByPkg := map[string]bool{}

	for _, name := range names {
		archivePath := opts.PackageArchives[name]
		hash := opts.PackageHashes[name]
		if hash == "" {
			// 本地计算归档摘要。
			sum, err := fileSHA256(archivePath)
			if err != nil {
				return nil, fmt.Errorf("计算包 %s 摘要失败: %w", name, err)
			}
			hash = sum
		}

		svcDir := filepath.Join(opts.WorkDir, "src", "generated", "services", name)
		if err := extractArchive(archivePath, svcDir); err != nil {
			return nil, fmt.Errorf("解包 SDK %s 失败: %w", name, err)
		}
		if err := RewriteHostImports(opts.WorkDir, svcDir); err != nil {
			return nil, fmt.Errorf("重写 SDK %s 的宿主 import 失败: %w", name, err)
		}

		mf, err := readManifest(svcDir)
		if err != nil {
			res.ValidationResults[name] = []string{err.Error()}
			return nil, fmt.Errorf("读取 SDK %s 的 manifest 失败: %w", name, err)
		}
		// 归档可能内嵌一层目录，manifest 需在根。若未在根找到则失败。
		if mf == nil {
			res.ValidationResults[name] = []string{"归档根目录缺少 manifest.json"}
			return nil, fmt.Errorf("SDK %s 归档根目录缺少 manifest.json", name)
		}

		if verrs := ValidateManifest(mf); len(verrs) > 0 {
			res.ValidationResults[name] = verrs
			return nil, fmt.Errorf("SDK %s 的 manifest 校验失败: %s", name, strings.Join(verrs, "; "))
		}
		res.ValidationResults[name] = nil
		res.ManifestSums[name] = hash
		if opts.RunScripts[name] {
			runScriptsByPkg[name] = true
		}

		res.SDKs = append(res.SDKs, &Sdk{
			Name:     name,
			Manifest: mf,
			SHA256:   hash,
			Dir:      svcDir,
			PkgDir:   filepath.ToSlash(filepath.Join("services", name)),
		})
		res.SDKList = append(res.SDKList, name)

		// 2. 合并依赖（deep merge：同名以本次包声明的为准；模板自有依赖随后合并）。
		for k, v := range mf.Dependencies {
			allDeps[k] = v
		}
	}

	// 3. 合并模板原有 dependencies。
	templatePkg, err := readPackageJSON(filepath.Join(opts.WorkDir, "package.json"))
	if err != nil {
		return nil, fmt.Errorf("读取模板 package.json 失败: %w", err)
	}
	for k, v := range templatePkg {
		if _, exists := allDeps[k]; !exists {
			allDeps[k] = v
		}
	}
	if err := writePackageJSON(filepath.Join(opts.WorkDir, "package.json"), allDeps); err != nil {
		return nil, fmt.Errorf("写回合并后的 package.json 失败: %w", err)
	}

	// 4. 生成四个文件。
	generatedDir := filepath.Join(opts.WorkDir, "src", "generated")
	if err := writeSDKManifest(generatedDir, res.SDKs); err != nil {
		return nil, err
	}
	if err := writeSDKRoutes(generatedDir, res.SDKs); err != nil {
		return nil, err
	}
	if err := writeSDKNavbar(generatedDir, res.SDKs); err != nil {
		return nil, err
	}
	if err := writeSDKDashboard(generatedDir, res.SDKs); err != nil {
		return nil, err
	}

	// 5. SDK 集合摘要。
	res.SDKVersion = fingerprintSDKs(res.SDKList, res.ManifestSums)
	res.RunScripts = runScriptsByPkg

	return res, nil
}

// fileSHA256 计算文件 sha256。
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractArchive 按后缀选择解包方式（tar.gz / zip）。
func extractArchive(path, destDir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractTarGz(f, destDir)
	case strings.HasSuffix(lower, ".zip"):
		info, err := f.Stat()
		if err != nil {
			return err
		}
		return extractZip(f, info.Size(), destDir)
	default:
		return fmt.Errorf("不支持的归档格式: %s（仅支持 tar.gz / zip）", path)
	}
}

// readManifest 读取解包目录根部的 manifest.json。
func readManifest(svcDir string) (*Manifest, error) {
	path := filepath.Join(svcDir, "manifest.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var mf Manifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return nil, fmt.Errorf("解析 manifest.json 失败: %w", err)
	}
	return &mf, nil
}

// readPackageJSON 读取 package.json 的 dependencies 部分。
func readPackageJSON(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, err
	}
	if pkg.Dependencies == nil {
		return map[string]string{}, nil
	}
	return pkg.Dependencies, nil
}

// writePackageJSON 把合并后的 dependencies 写回 package.json。
// 保留模板 package.json 的其他字段，仅替换 dependencies。
func writePackageJSON(path string, deps map[string]string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var pkg map[string]any
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return err
	}
	if pkg == nil {
		pkg = map[string]any{}
	}
	depMap := map[string]any{}
	for k, v := range deps {
		depMap[k] = v
	}
	pkg["dependencies"] = depMap
	out, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// fingerprintSDKs 计算 SDK 集合摘要（name+sha256 排序后哈希），用作状态记录。
func fingerprintSDKs(names []string, sums map[string]string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, n+"="+sums[n])
	}
	sort.Strings(parts)
	h := sha256.New()
	_, _ = h.Write([]byte(strings.Join(parts, ",")))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
