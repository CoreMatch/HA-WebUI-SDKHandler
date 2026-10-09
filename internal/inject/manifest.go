// Package inject 负责把 SDK 源码头包注入前端模板，生成聚合前端。
//
// 流程（每次构建全量执行，不做增量）：
//  1. 把只读模板目录 template_dir 全量复制到本次构建的 work_dir。
//  2. 解包每个 SDK 归档到 work_dir/src/generated/services/<name>/，
//     解包时做路径安全校验（拒绝 ..、绝对路径、symlink/hardlink）。
//  3. 校验各包 manifest.json（schema_version=1、name 合法、routes 非空等），
//     记录校验结果。
//  4. 合并所有 manifest.dependencies 到 work_dir/package.json（deep merge）。
//  5. 在 work_dir/src/generated/ 下生成四个文件：
//     sdk-manifest.ts / sdk-routes.tsx / sdk-navbar.tsx / sdk-dashboard.tsx。
package inject

import (
	"fmt"
	"regexp"
	"strings"
)

// SchemaVersion 是当前支持的 manifest 版本。
const SchemaVersion = 1

// Manifest 对应 SDK 包根目录的 manifest.json，字段与契约一一对应。
type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Routes        []RouteEntry      `json:"routes"`
	Menu          *MenuEntry        `json:"menu,omitempty"`
	Dashboard     *MenuEntry        `json:"dashboard,omitempty"`
	Dependencies  map[string]string `json:"dependencies,omitempty"`
	RunScripts    bool              `json:"run_scripts"`
}

// RouteEntry 是 manifest.routes 中的一项。
type RouteEntry struct {
	Path   string `json:"path"`
	Module string `json:"module"`
	Title  string `json:"title,omitempty"`
}

// MenuEntry 是 manifest.menu / manifest.dashboard 项（两者结构相同）。
type MenuEntry struct {
	Label string `json:"label"`
	Path  string `json:"path"`
}

// namePattern 与契约一致：`[a-z0-9]([a-z0-9-]*[a-z0-9])?`，最长 64 字符。
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// versionPattern 与契约一致：semantic version `major.minor.patch`。
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// ValidateManifest 校验 manifest 是否满足契约（纯函数，供单测直接调用）。
// 返回错误列表；为空表示通过。rootPath 为包内根目录在生成目录中的相对路径
// （如 services/foo），用于 module 必须位于包内、menu/dashboard.path 必须
// 引用本包 routes 的校验。
func ValidateManifest(m *Manifest) []string {
	var errs []string

	if m.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Sprintf("schema_version 必须为 %d", SchemaVersion))
	}
	if !namePattern.MatchString(m.Name) || len(m.Name) > 64 {
		errs = append(errs, "name 必须匹配 [a-z0-9]([a-z0-9-]*[a-z0-9])? 且不超过 64 字符")
	}
	if !versionPattern.MatchString(m.Version) {
		errs = append(errs, "version 必须为 major.minor.patch 语义版本")
	}
	if len(m.Routes) == 0 {
		errs = append(errs, "routes 不能为空")
	}
	for i, r := range m.Routes {
		if err := validateRoutePath(r.Path); err != "" {
			errs = append(errs, fmt.Sprintf("routes[%d].path 非法: %s", i, err))
		}
		if err := validateModulePath(r.Module); err != "" {
			errs = append(errs, fmt.Sprintf("routes[%d].module 非法: %s", i, err))
		}
	}
	if m.Menu != nil {
		if strings.TrimSpace(m.Menu.Label) == "" {
			errs = append(errs, "menu.label 必填")
		}
		if !routePathExists(m.Routes, m.Menu.Path) {
			errs = append(errs, "menu.path 必须引用本包 routes 中的路径")
		}
	}
	if m.Dashboard != nil {
		if strings.TrimSpace(m.Dashboard.Label) == "" {
			errs = append(errs, "dashboard.label 必填")
		}
		if !routePathExists(m.Routes, m.Dashboard.Path) {
			errs = append(errs, "dashboard.path 必须引用本包 routes 中的路径")
		}
	}
	return errs
}

// validateRoutePath 校验路由路径规则（契约）：
// 必须以 / 开头；除根路径 / 外不得以 / 结尾；不得含 ? / # / .. 段。
func validateRoutePath(p string) string {
	if p == "" {
		return "不能为空"
	}
	if !strings.HasPrefix(p, "/") {
		return "必须以 / 开头"
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		return "不得以 / 结尾（根路径 / 除外）"
	}
	if strings.ContainsAny(p, "?#") {
		return "不得包含 ? 或 #"
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "不得包含 .. 段"
		}
	}
	return ""
}

// validateModulePath 校验模块相对路径：包内相对路径，无 ..、无绝对路径。
func validateModulePath(m string) string {
	if m == "" {
		return "不能为空"
	}
	if strings.HasPrefix(m, "/") {
		return "必须是包内相对路径，不得为绝对路径"
	}
	if strings.HasPrefix(m, "./") {
		// 允许以 ./ 开头，但继续检查段。
		m = strings.TrimPrefix(m, "./")
	}
	for _, seg := range strings.Split(m, "/") {
		if seg == ".." {
			return "不得包含 .. 段（必须位于包内）"
		}
	}
	return ""
}

func routePathExists(routes []RouteEntry, p string) bool {
	for _, r := range routes {
		if r.Path == p {
			return true
		}
	}
	return false
}
