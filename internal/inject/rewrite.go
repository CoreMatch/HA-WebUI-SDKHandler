package inject

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	fromSpecifierPattern       = regexp.MustCompile(`\bfrom\s+(['"])([^'"]+)(['"])`)
	sideEffectImportPattern    = regexp.MustCompile(`\bimport\s+(['"])([^'"]+)(['"])`)
	dynamicImportPattern       = regexp.MustCompile(`\bimport\s*\(\s*(['"])([^'"]+)(['"])\s*\)`)
	rewriteCandidateExtensions = []string{"", ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"}
)

// RewriteHostImports rewrites SDK imports that escape the package directory but
// actually refer to files provided by the host application's src/ tree.
func RewriteHostImports(workDir, svcDir string) error {
	return filepath.WalkDir(svcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ts", ".tsx", ".js", ".jsx", ".mts", ".cts":
			return rewriteHostImportsInFile(workDir, svcDir, path)
		default:
			return nil
		}
	})
}

func rewriteHostImportsInFile(workDir, svcDir, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取源码文件失败: %w", err)
	}

	updated := string(raw)
	updated = rewriteImportMatches(updated, fromSpecifierPattern, workDir, svcDir, path)
	updated = rewriteImportMatches(updated, sideEffectImportPattern, workDir, svcDir, path)
	updated = rewriteImportMatches(updated, dynamicImportPattern, workDir, svcDir, path)

	if updated == string(raw) {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("写回源码文件失败: %w", err)
	}
	return nil
}

func rewriteImportMatches(content string, pattern *regexp.Regexp, workDir, svcDir, path string) string {
	return pattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := pattern.FindStringSubmatch(match)
		if len(parts) < 3 {
			return match
		}
		rewritten := rewriteHostImportSpecifier(workDir, svcDir, path, parts[2])
		if rewritten == parts[2] {
			return match
		}
		return strings.Replace(match, parts[2], rewritten, 1)
	})
}

func rewriteHostImportSpecifier(workDir, svcDir, filePath, spec string) string {
	if !strings.HasPrefix(spec, ".") {
		return spec
	}

	fileDir := filepath.Dir(filePath)
	target := filepath.Clean(filepath.Join(fileDir, filepath.FromSlash(spec)))
	if moduleExists(target) || isWithin(svcDir, target) {
		return spec
	}

	suffix := trimRelativePrefix(spec)
	if suffix == "" || strings.HasPrefix(suffix, ".") {
		return spec
	}

	hostTarget := filepath.Join(workDir, "src", filepath.FromSlash(suffix))
	if !moduleExists(hostTarget) {
		return spec
	}

	rel, err := filepath.Rel(fileDir, hostTarget)
	if err != nil {
		return spec
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}

func trimRelativePrefix(spec string) string {
	s := filepath.ToSlash(spec)
	for {
		switch {
		case strings.HasPrefix(s, "../"):
			s = strings.TrimPrefix(s, "../")
		case strings.HasPrefix(s, "./"):
			s = strings.TrimPrefix(s, "./")
		default:
			return s
		}
	}
}

func moduleExists(base string) bool {
	for _, ext := range rewriteCandidateExtensions {
		if fileExists(base + ext) {
			return true
		}
	}
	for _, ext := range rewriteCandidateExtensions[1:] {
		if fileExists(filepath.Join(base, "index"+ext)) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
