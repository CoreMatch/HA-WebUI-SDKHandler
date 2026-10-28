package inject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSDKRoutesPlacesRouteAtTopLevel(t *testing.T) {
	dir := t.TempDir()
	err := writeSDKRoutes(dir, []*Sdk{
		{
			Name: "demo-sdk",
			Manifest: &Manifest{
				Name:    "demo-sdk",
				Version: "1.0.0",
				Routes: []RouteEntry{
					{
						Path:   "/example",
						Module: "pages/Example",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("writeSDKRoutes returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "sdk-routes.tsx"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	got := string(raw)

	if strings.Contains(got, "<Suspense key=") {
		t.Fatalf("sdkRouteElements should not wrap <Route> with top-level <Suspense>:\n%s", got)
	}
	if !strings.Contains(got, "export const sdkRouteElements: ReactNode[] = [\n  <Route\n") {
		t.Fatalf("sdkRouteElements should emit top-level <Route> entries:\n%s", got)
	}
	if !strings.Contains(got, "element={\n      <Suspense fallback={<SdkRouteFallback />}>\n        <SdkRouteDemoSdk0 />\n      </Suspense>\n    }\n  />,") {
		t.Fatalf("generated route should place Suspense inside Route element:\n%s", got)
	}
}

func TestWriteSDKNavbarMultiPage(t *testing.T) {
	dir := t.TempDir()
	err := writeSDKNavbar(dir, []*Sdk{
		{
			Name: "multi-svc",
			Manifest: &Manifest{
				Name:    "multi-svc",
				Version: "1.0.0",
				Routes: []RouteEntry{
					{Path: "/p1", Module: "src/p1"},
					{Path: "/p2", Module: "src/p2"},
				},
				Menu: []MenuEntry{
					{Label: "P1", Path: "/p1"},
					{Label: "P2", Path: "/p2"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("writeSDKNavbar returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "sdk-navbar.tsx"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, `key: "multi-svc-0"`) || !strings.Contains(got, `label: "P1"`) {
		t.Errorf("missing first menu item: %s", got)
	}
	if !strings.Contains(got, `key: "multi-svc-1"`) || !strings.Contains(got, `label: "P2"`) {
		t.Errorf("missing second menu item: %s", got)
	}
}

func TestWriteSDKDashboardMultiPage(t *testing.T) {
	dir := t.TempDir()
	err := writeSDKDashboard(dir, []*Sdk{
		{
			Name: "multi-svc",
			Manifest: &Manifest{
				Name:    "multi-svc",
				Version: "1.0.0",
				Routes: []RouteEntry{
					{Path: "/p1", Module: "src/p1"},
					{Path: "/p2", Module: "src/p2"},
				},
				Dashboard: []MenuEntry{
					{Label: "D1", Path: "/p1"},
					{Label: "D2", Path: "/p2"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("writeSDKDashboard returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "sdk-dashboard.tsx"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, "const SdkDashMultiSvc0 = lazy") || !strings.Contains(got, "const SdkDashMultiSvc1 = lazy") {
		t.Errorf("missing lazy components: %s", got)
	}
	if !strings.Contains(got, `key: "multi-svc-0"`) || !strings.Contains(got, `label: "D1"`) {
		t.Errorf("missing first dashboard item: %s", got)
	}
	if !strings.Contains(got, `key: "multi-svc-1"`) || !strings.Contains(got, `label: "D2"`) {
		t.Errorf("missing second dashboard item: %s", got)
	}
}

func TestWriteSDKCriticalOperations(t *testing.T) {
	dir := t.TempDir()
	err := writeSDKCriticalOperations(dir, []*Sdk{
		{
			Name: "security-tools",
			Manifest: &Manifest{
				CriticalOperations: []MenuEntry{
					{Label: `Reset "all" keys`, Path: "/security/keys"},
					{Label: "Delete account", Path: "/security/delete"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("writeSDKCriticalOperations returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "sdk-critical-operations.ts"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "import type { SdkCriticalOperationItem } from '../types/generated-sdk';") ||
		!strings.Contains(got, `key: "security-tools-critical-operation-0"`) ||
		!strings.Contains(got, `label: "Reset \"all\" keys"`) ||
		!strings.Contains(got, `path: "/security/keys"`) {
		t.Fatalf("generated critical operation is missing or malformed:\n%s", got)
	}
	if !strings.Contains(got, `key: "security-tools-critical-operation-1"`) ||
		!strings.Contains(got, `label: "Delete account"`) {
		t.Fatalf("second critical operation is missing:\n%s", got)
	}
}

func TestWriteSDKProfileCardsSortMetadata(t *testing.T) {
	dir := t.TempDir()
	high := 9
	err := writeSDKProfileCards(dir, []*Sdk{
		{
			Name: "alpha-svc",
			Manifest: &Manifest{
				Name:    "alpha-svc",
				Version: "1.0.0",
				ProfileCards: []ProfileCardEntry{
					{Title: "Alpha A", Module: "src/profile/AlphaA", Priority: &high},
					{Title: "Alpha B", Module: "src/profile/AlphaB"},
				},
			},
		},
		{
			Name: "beta-svc",
			Manifest: &Manifest{
				Name:    "beta-svc",
				Version: "1.0.0",
				ProfileCards: []ProfileCardEntry{
					{Title: "Beta A", Module: "src/profile/BetaA"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("writeSDKProfileCards returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "sdk-profile-cards.tsx"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, "const SdkProfileCardAlphaSvc0 = lazy") || !strings.Contains(got, "const SdkProfileCardBetaSvc0 = lazy") {
		t.Errorf("missing lazy components: %s", got)
	}
	if !strings.Contains(got, `title: "Alpha A"`) || !strings.Contains(got, `priority: 9`) {
		t.Errorf("missing explicit priority card: %s", got)
	}
	if !strings.Contains(got, `loadOrder: 0`) || !strings.Contains(got, `loadOrder: 1`) || !strings.Contains(got, `loadOrder: 2`) {
		t.Errorf("missing stable loadOrder sequence: %s", got)
	}
}

func TestWriteSDKEmpty(t *testing.T) {
	dir := t.TempDir()

	// 1. Test empty routes
	if err := writeSDKRoutes(dir, []*Sdk{}); err != nil {
		t.Fatalf("writeSDKRoutes returned error: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "sdk-routes.tsx"))
	got := string(raw)
	if strings.Contains(got, "import { lazy, Suspense } from 'react'") {
		t.Errorf("sdk-routes.tsx should not contain lazy import when empty:\n%s", got)
	}
	if strings.Contains(got, "function SdkRouteFallback()") {
		t.Errorf("sdk-routes.tsx should not contain fallback when empty:\n%s", got)
	}

	// 2. Test empty dashboard
	if err := writeSDKDashboard(dir, []*Sdk{}); err != nil {
		t.Fatalf("writeSDKDashboard returned error: %v", err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "sdk-dashboard.tsx"))
	got = string(raw)
	if strings.Contains(got, "import { lazy } from 'react'") {
		t.Errorf("sdk-dashboard.tsx should not contain lazy import when empty:\n%s", got)
	}

	// 3. Test empty profile cards
	if err := writeSDKProfileCards(dir, []*Sdk{}); err != nil {
		t.Fatalf("writeSDKProfileCards returned error: %v", err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "sdk-profile-cards.tsx"))
	got = string(raw)
	if strings.Contains(got, "import { lazy } from 'react'") {
		t.Errorf("sdk-profile-cards.tsx should not contain lazy import when empty:\n%s", got)
	}

	// 4. Empty Critical Operations
	if err := writeSDKCriticalOperations(dir, []*Sdk{}); err != nil {
		t.Fatalf("writeSDKCriticalOperations returned error: %v", err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "sdk-critical-operations.ts"))
	got = string(raw)
	if !strings.Contains(got, "export const sdkCriticalOperations: SdkCriticalOperationItem[] = [\n];") {
		t.Errorf("empty critical operations output should contain an empty array:\n%s", got)
	}
}
