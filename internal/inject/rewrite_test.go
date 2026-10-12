package inject

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteHostImportsRepointsHostUtilities(t *testing.T) {
	workDir := t.TempDir()
	svcDir := filepath.Join(workDir, "src", "generated", "services", "yggdrasil-api")

	if err := os.MkdirAll(filepath.Join(workDir, "src", "utils"), 0o755); err != nil {
		t.Fatalf("create host utils dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "src", "utils", "config.ts"), []byte("export const BackendUrl = '';\n"), 0o644); err != nil {
		t.Fatalf("write host config: %v", err)
	}

	proxyPath := filepath.Join(svcDir, "src", "utils", "proxy.ts")
	if err := os.MkdirAll(filepath.Dir(proxyPath), 0o755); err != nil {
		t.Fatalf("create sdk utils dir: %v", err)
	}
	const before = "export { BackendUrl } from '../../../utils/config';\n"
	if err := os.WriteFile(proxyPath, []byte(before), 0o644); err != nil {
		t.Fatalf("write proxy file: %v", err)
	}

	if err := RewriteHostImports(workDir, svcDir); err != nil {
		t.Fatalf("RewriteHostImports returned error: %v", err)
	}

	raw, err := os.ReadFile(proxyPath)
	if err != nil {
		t.Fatalf("read rewritten proxy file: %v", err)
	}

	got := string(raw)
	if !strings.Contains(got, "../../../../../utils/config") {
		t.Fatalf("expected proxy import to point to host utils after injection, got:\n%s", got)
	}
}

func TestRewriteHostImportsKeepsInPackageImports(t *testing.T) {
	workDir := t.TempDir()
	svcDir := filepath.Join(workDir, "src", "generated", "services", "demo-sdk")

	helperPath := filepath.Join(svcDir, "src", "utils", "helper.ts")
	proxyPath := filepath.Join(svcDir, "src", "pages", "Example.ts")

	if err := os.MkdirAll(filepath.Dir(helperPath), 0o755); err != nil {
		t.Fatalf("create helper dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(proxyPath), 0o755); err != nil {
		t.Fatalf("create page dir: %v", err)
	}
	if err := os.WriteFile(helperPath, []byte("export const helper = true;\n"), 0o644); err != nil {
		t.Fatalf("write helper file: %v", err)
	}
	const before = "import { helper } from '../utils/helper';\n"
	if err := os.WriteFile(proxyPath, []byte(before), 0o644); err != nil {
		t.Fatalf("write page file: %v", err)
	}

	if err := RewriteHostImports(workDir, svcDir); err != nil {
		t.Fatalf("RewriteHostImports returned error: %v", err)
	}

	raw, err := os.ReadFile(proxyPath)
	if err != nil {
		t.Fatalf("read page file: %v", err)
	}
	if string(raw) != before {
		t.Fatalf("expected in-package import to stay unchanged, got:\n%s", string(raw))
	}
}
