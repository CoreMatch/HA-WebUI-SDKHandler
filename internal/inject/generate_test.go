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
