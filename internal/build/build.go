// Package build 编排 npm install → tsc -b → vite build 的构建管线。
//
// 构建策略（契约 Build Policy）：
//   - npm install 默认带 --ignore-scripts；仅当包声明 run_scripts:true 时执行脚本。
//   - 配置了 npm 黑名单（精确包名）时：任一 SDK 包或模板声明黑名单包 → 本次构建直接失败。
//   - 任一步失败或超时（默认 20 分钟）→ 标记构建失败，删除本次 work 目录产物，
//     保留上一版 current 不动。
package build

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// StepResult 是单步命令的执行结果。
type StepResult struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	OK      bool   `json:"ok"`
	Log     string `json:"log"` // 截断后的 stdout+stderr
	Err     string `json:"error,omitempty"`
}

// Options 是构建管线输入。
type Options struct {
	// WorkDir 是注入后的前端工作目录（必须已含完整模板 + 注入产物）。
	WorkDir string
	// NPMCommand 是 npm 可执行文件（默认 npm）。
	NPMCommand string
	// IgnoreScriptsDefault 是否默认 --ignore-scripts（契约默认 true）。
	IgnoreScriptsDefault bool
	// RunScripts 为 true 的包名集合（run_scripts:true 声明）。
	RunScripts map[string]bool
	// NPMBlacklist 是精确包名黑名单；命中即拒绝本次构建。
	NPMBlacklist []string
	// Timeout 是整次构建的超时。
	Timeout time.Duration
	// Logf 可选：逐步日志输出。
	Logf func(format string, args ...any)
}

// Result 是构建管线结果。
type Result struct {
	OK    bool         `json:"ok"`
	Steps []StepResult `json:"steps"`
	Err   string       `json:"error,omitempty"`
}

// DefaultTimeout 是整次构建默认超时（20 分钟）。
const DefaultTimeout = 20 * time.Minute

// Run 顺序执行 npm install → tsc -b → vite build。
func Run(opts Options) *Result {
	res := &Result{OK: true}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	// 0. 黑名单预检：发生在任何副作用之前，直接 fail 该次构建。
	if err := checkBlacklist(opts); err != nil {
		res.OK = false
		res.Err = err.Error()
		opts.Logf("[build] 黑名单预检失败: %v", err)
		return res
	}

	// 1. npm install
	// 默认 --ignore-scripts；若存在 run_scripts:true 的包，则本次允许执行
	// 依赖树的生命周期脚本（契约：unless the package declares run_scripts:true）。
	ignoreScripts := opts.IgnoreScriptsDefault
	for _, approved := range opts.RunScripts {
		if approved {
			ignoreScripts = false
			opts.Logf("[build] 存在 run_scripts:true 的 SDK 包，本次 npm install 允许执行生命周期脚本")
			break
		}
	}
	var installArgs []string
	installArgs = append(installArgs, "install")
	if ignoreScripts {
		installArgs = append(installArgs, "--ignore-scripts")
	}
	if sr := runStep(ctx, opts, "npm install", opts.NPMCommand, installArgs...); !sr.OK {
		opts.Logf("[build] npm install 失败")
			if sr.Log != "" {
				opts.Logf("[build] npm install 输出:\n%s", sr.Log)
			}
		res.OK = false
		res.Steps = append(res.Steps, sr)
		res.Err = sr.Err
		return res
	} else {
		res.Steps = append(res.Steps, sr)
	}

	// 2. 模板若定义了 build 脚本，优先用 npm run build（其本身须包含 tsc+vite）；
	//    否则显式执行 npx/npm tsc -b && vite build。
	//    这里读取模板 package.json（注入阶段不修改 scripts），决定走哪条路径。
	useNpmBuild := templateHasBuildScript(filepath.Join(opts.WorkDir, "package.json"))
	if useNpmBuild {
		if sr := runStep(ctx, opts, "npm run build", opts.NPMCommand, "run", "build"); !sr.OK {
			opts.Logf("[build] npm run build 失败")
				if sr.Log != "" {
					opts.Logf("[build] npm run build 输出:\n%s", sr.Log)
				}
			res.OK = false
			res.Steps = append(res.Steps, sr)
			res.Err = sr.Err
			return res
		} else {
			res.Steps = append(res.Steps, sr)
		}
	} else {
		// 显式 tsc -b 与 vite build。
		tsc := opts.NPMCommand
		if sr := runStep(ctx, opts, "tsc -b", tsc, "exec", "tsc", "-b"); !sr.OK {
			opts.Logf("[build] tsc -b 失败")
				if sr.Log != "" {
					opts.Logf("[build] tsc -b 输出:\n%s", sr.Log)
				}
			res.OK = false
			res.Steps = append(res.Steps, sr)
			res.Err = sr.Err
			return res
		} else {
			res.Steps = append(res.Steps, sr)
		}
		if sr := runStep(ctx, opts, "vite build", opts.NPMCommand, "exec", "vite", "build"); !sr.OK {
			opts.Logf("[build] vite build 失败")
				if sr.Log != "" {
					opts.Logf("[build] vite build 输出:\n%s", sr.Log)
				}
			res.OK = false
			res.Steps = append(res.Steps, sr)
			res.Err = sr.Err
			return res
		} else {
			res.Steps = append(res.Steps, sr)
		}
	}
	return res
}

// checkBlacklist 检查合并后的依赖中是否命中黑名单。
// 命中返回错误（该次构建直接失败）；否则 nil。
func checkBlacklist(opts Options) error {
	if len(opts.NPMBlacklist) == 0 {
		return nil
	}
	black := make(map[string]bool, len(opts.NPMBlacklist))
	for _, p := range opts.NPMBlacklist {
		black[strings.TrimSpace(p)] = true
	}

	pkgPath := filepath.Join(opts.WorkDir, "package.json")
	deps, err := readDependencies(pkgPath)
	if err != nil {
		return fmt.Errorf("读取合并后 package.json 依赖失败: %w", err)
	}
	var hits []string
	for name := range deps {
		if black[name] {
			hits = append(hits, name)
		}
	}
	if len(hits) > 0 {
		return fmt.Errorf("依赖命中 npm 黑名单: %s（本次构建被拒绝）", strings.Join(hits, ", "))
	}
	return nil
}

// readDependencies 读取 package.json 中 dependencies 的包名集合。
func readDependencies(pkgPath string) (map[string]string, error) {
	raw, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil, err
	}
	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := jsonUnmarshal(raw, &pkg); err != nil {
		return nil, err
	}
	if pkg.Dependencies == nil {
		return map[string]string{}, nil
	}
	return pkg.Dependencies, nil
}

// templateHasBuildScript 判断模板 package.json 是否定义了 build 脚本。
func templateHasBuildScript(pkgPath string) bool {
	raw, err := os.ReadFile(pkgPath)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := jsonUnmarshal(raw, &pkg); err != nil {
		return false
	}
	build, ok := pkg.Scripts["build"]
	if !ok {
		return false
	}
	// 安全判定：build 脚本必须包含 tsc 与 vite（契约要求 tsc -b + vite build）。
	lower := strings.ToLower(build)
	return strings.Contains(lower, "tsc") && strings.Contains(lower, "vite")
}

// runStep 执行一步命令，捕获 stdout/stderr，超时与失败均返回非 OK。
func runStep(ctx context.Context, opts Options, name, command string, args ...string) StepResult {
	var buf bytes.Buffer
	opts.Logf("[build] 开始 %s: %s %s", name, command, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = opts.WorkDir
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		stepErr := "命令执行失败"
		if ctx.Err() == context.DeadlineExceeded {
			stepErr = fmt.Sprintf("命令超时（超过 %s）", opts.Timeout)
		} else if exit, ok := err.(*exec.ExitError); ok {
			stepErr = fmt.Sprintf("命令退出码 %d", exit.ExitCode())
		}
		return StepResult{
			Name:    name,
			Command: command + " " + strings.Join(args, " "),
			OK:      false,
			Log:     truncate(buf.String(), 4096),
			Err:     stepErr,
		}
	}
	return StepResult{
		Name:    name,
		Command: command + " " + strings.Join(args, " "),
		OK:      true,
		Log:     truncate(buf.String(), 4096),
	}
}

// NewBuildID 生成 build-id：时间戳 + 短随机 hash。
// 形如 20261009-153045-a1b2c3d4。
func NewBuildID(now time.Time) string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%s-%s",
		now.UTC().Format("20060102-150405"),
		hex.EncodeToString(buf))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// jsonUnmarshal 避免在 build 包引入多余依赖（直接标准库即可）。
func jsonUnmarshal(raw []byte, v any) error { return json.Unmarshal(raw, v) }
