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
