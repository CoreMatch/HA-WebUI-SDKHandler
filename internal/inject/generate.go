package inject

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeGenerated 写入一个生成文件。
func writeGenerated(dir, filename, content string) error {
	target := filepath.Join(dir, filename)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

// jsString 生成合法的双引号 TS/JS 字符串字面量（转义 \ 与 "）。
func jsString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// moduleRef 生成包内 module 的相对引用路径：
// 生成目录 src/generated/sdk-*.tsx 位于 src/generated/ 下，
// 而包解包在 src/generated/services/<name>/<module>，
// 因此相对路径为 ./services/<name>/<module 路径>。
// module 无扩展名，靠 Vite resolve.extensions / TS bundler 解析。
func moduleRef(pkgName, module string) string {
	module = strings.TrimPrefix(module, "./")
	return "./services/" + pkgName + "/" + module
}

// writeSDKManifest 生成 sdk-manifest.ts：
// `export const sdkManifest: SdkManifestEntry[] = [...]`，
// 类型来自 src/types/generated-sdk.ts（模板已有，人工维护）。
func writeSDKManifest(dir string, sdks []*Sdk) error {
	var b strings.Builder
	b.WriteString("// 本文件由 HA-WebUI-SDKHandler 自动生成，请勿手工修改。\n")
	b.WriteString("// 类型契约见 src/types/generated-sdk.ts 与 HA-Contract/docs/dev/HRPAuth/sdk-package.md。\n")
	b.WriteString("import type { SdkManifestEntry } from '../types/generated-sdk';\n\n")
	b.WriteString("export const sdkManifest: SdkManifestEntry[] = [\n")
	for _, sdk := range sdks {
		m := sdk.Manifest
		b.WriteString("  {\n")
		b.WriteString("    name: " + jsString(m.Name) + ",\n")
		b.WriteString("    version: " + jsString(m.Version) + ",\n")
		b.WriteString("    routes: [\n")
		for _, r := range m.Routes {
			b.WriteString("      { path: " + jsString(r.Path) + ", module: " + jsString(r.Module))
			if r.Title != "" {
				b.WriteString(", title: " + jsString(r.Title))
			}
			b.WriteString(" },\n")
		}
		b.WriteString("    ],\n")
		if m.Menu != nil {
			b.WriteString("    menu: { label: " + jsString(m.Menu.Label) + ", path: " + jsString(m.Menu.Path) + " },\n")
		}
		if m.Dashboard != nil {
			b.WriteString("    dashboard: { label: " + jsString(m.Dashboard.Label) + ", path: " + jsString(m.Dashboard.Path) + " },\n")
		}
		b.WriteString("    runScripts: " + boolLit(sdkManifestRunScripts(m)) + ",\n")
		b.WriteString("    sha256: " + jsString(sdk.SHA256) + ",\n")
		b.WriteString("  },\n")
	}
	b.WriteString("];\n")
	return writeGenerated(dir, "sdk-manifest.ts", b.String())
}

// sdkManifestRunScripts 返回 manifest 中 run_scripts 的值（默认为 false）。
func sdkManifestRunScripts(m *Manifest) bool { return m.RunScripts }

func boolLit(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// writeSDKRoutes 生成 sdk-routes.tsx：
//   - 每个 route 用 React.lazy 懒加载（module 默认导出 React 组件）。
//   - 导出 sdkRouteElements: React.ReactNode[]（每个为 Suspense 包裹的 <Route>），
//     以及 sdkRoutes: RouteObject[]。
//
// 遵守模板 TS 严格模式：import type、无未使用变量。
func writeSDKRoutes(dir string, sdks []*Sdk) error {
	var b strings.Builder
	b.WriteString("// 本文件由 HA-WebUI-SDKHandler 自动生成，请勿手工修改。\n")
	b.WriteString("// 路由采用 React.lazy 按需加载，Suspense 包裹。<Route> 直接使用绝对路径，\n")
	b.WriteString("// 可挂载在任意层级；sdkRoutes 为 React Router 的 RouteObject 形态。\n")
	b.WriteString("import { lazy, Suspense } from 'react';\n")
	b.WriteString("import type { ReactNode } from 'react';\n")
	b.WriteString("import type { RouteObject } from 'react-router-dom';\n")
	b.WriteString("import { Route } from 'react-router-dom';\n\n")
	b.WriteString("function SdkRouteFallback() {\n")
	b.WriteString("  return <span>Loading...</span>;\n")
	b.WriteString("}\n\n")

	for _, sdk := range sdks {
		for i, r := range sdk.Manifest.Routes {
			// 组件名：服务名+序号，保证唯一且为合法标识符。
			ident := sdk.Name + "_" + fmt.Sprintf("%d", i)
			b.WriteString("const " + ident + " = lazy(() => import(" + jsString(moduleRef(sdk.Name, r.Module)) + "));\n")
		}
	}
	b.WriteString("\n")

	// sdkRouteElements：ReactNode 数组，每个为 Suspense 包裹的 <Route>。
	b.WriteString("export const sdkRouteElements: ReactNode[] = [\n")
	for _, sdk := range sdks {
		for i, r := range sdk.Manifest.Routes {
			ident := sdk.Name + "_" + fmt.Sprintf("%d", i)
			b.WriteString("  <Suspense key=" + jsString(sdk.Name+"-"+r.Path) + " fallback={<SdkRouteFallback />}>\n")
			b.WriteString("    <Route path=" + jsString(r.Path) + " element={<" + ident + " />} />\n")
			b.WriteString("  </Suspense>,\n")
		}
	}
	b.WriteString("];\n\n")

	// sdkRoutes：RouteObject 数组。
	b.WriteString("export const sdkRoutes: RouteObject[] = [\n")
	for _, sdk := range sdks {
		for i, r := range sdk.Manifest.Routes {
			ident := sdk.Name + "_" + fmt.Sprintf("%d", i)
			b.WriteString("  {\n")
			b.WriteString("    path: " + jsString(r.Path) + ",\n")
			b.WriteString("    element: (\n")
			b.WriteString("      <Suspense fallback={<SdkRouteFallback />}>\n")
			b.WriteString("        <" + ident + " />\n")
			b.WriteString("      </Suspense>\n")
			b.WriteString("    ),\n")
			b.WriteString("  },\n")
		}
	}
	b.WriteString("];\n")
	return writeGenerated(dir, "sdk-routes.tsx", b.String())
}

// writeSDKNavbar 生成 sdk-navbar.tsx：
// `export const sdkNavbarItems: SdkNavbarItem[]`（key/label/path）。
func writeSDKNavbar(dir string, sdks []*Sdk) error {
	var b strings.Builder
	b.WriteString("// 本文件由 HA-WebUI-SDKHandler 自动生成，请勿手工修改。\n")
	b.WriteString("import type { SdkNavbarItem } from '../types/generated-sdk';\n\n")
	b.WriteString("export const sdkNavbarItems: SdkNavbarItem[] = [\n")
	for _, sdk := range sdks {
		m := sdk.Manifest
		if m.Menu == nil {
			continue
		}
		b.WriteString("  {\n")
		b.WriteString("    key: " + jsString(m.Name) + ",\n")
		b.WriteString("    label: " + jsString(m.Menu.Label) + ",\n")
		b.WriteString("    path: " + jsString(m.Menu.Path) + ",\n")
		b.WriteString("  },\n")
	}
	b.WriteString("];\n")
	return writeGenerated(dir, "sdk-navbar.tsx", b.String())
}

// writeSDKDashboard 生成 sdk-dashboard.tsx：
// `export const sdkDashboardItems: SdkDashboardItem[]`（key/label/element，
// element 是懒加载组件实例）。模块引用基于包内 module 路径，惰性求值。
func writeSDKDashboard(dir string, sdks []*Sdk) error {
	var b strings.Builder
	b.WriteString("// 本文件由 HA-WebUI-SDKHandler 自动生成，请勿手工修改。\n")
	b.WriteString("import { lazy } from 'react';\n")
	b.WriteString("import type { SdkDashboardItem } from '../types/generated-sdk';\n\n")

	// 先为需要 dashboard 的 SDK 生成 lazy 组件，避免 IIFE。
	b.WriteString("// 懒加载的 dashboard 路由组件（按服务声明）。\n")
	for _, sdk := range sdks {
		m := sdk.Manifest
		if m.Dashboard == nil {
			continue
		}
		mod := moduleForPath(m, m.Dashboard.Path)
		if mod == "" {
			// dashboard.path 已校验引用本包 routes；防御性兜底。
			continue
		}
		ident := sdk.Name + "_dash"
		b.WriteString("const " + ident + " = lazy(() => import(" + jsString(moduleRef(m.Name, mod)) + "));\n")
	}
	b.WriteString("\n")
	b.WriteString("export const sdkDashboardItems: SdkDashboardItem[] = [\n")
	for _, sdk := range sdks {
		m := sdk.Manifest
		if m.Dashboard == nil {
			continue
		}
		mod := moduleForPath(m, m.Dashboard.Path)
		if mod == "" {
			continue
		}
		ident := sdk.Name + "_dash"
		b.WriteString("  {\n")
		b.WriteString("    key: " + jsString(m.Name) + ",\n")
		b.WriteString("    label: " + jsString(m.Dashboard.Label) + ",\n")
		b.WriteString("    element: <" + ident + " />,\n")
		b.WriteString("  },\n")
	}
	b.WriteString("];\n")
	return writeGenerated(dir, "sdk-dashboard.tsx", b.String())
}

// moduleForPath 返回 manifest 中匹配指定路由 path 的 module 路径。
func moduleForPath(m *Manifest, path string) string {
	for _, r := range m.Routes {
		if r.Path == path {
			return r.Module
		}
	}
	return ""
}
