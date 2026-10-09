package inject

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// copyExcluded 是复制模板时需要排除的目录/文件名集合：
// 依赖、版本控制、构建产物、缓存与本次工作目录本身。
var copyExcluded = map[string]bool{
	"node_modules":    true,
	".git":            true,
	"dist":            true,
	"frontend-build":  true,
	"artifacts":       true,
	"state":           true,
	"current":         true,
	".next":           true,
	".turbo":          true,
	"out":             true,
	".DS_Store":       true,
	"Thumbs.db":       true,
}

// CopyTemplate 把模板目录全量复制到 workDir。
// 模板只读：每次构建全量复制，SDK 全量重新注入，不留增量残留。
// 复制时跳过排除集合中的条目。
func CopyTemplate(templateDir, workDir string) error {
	info, err := os.Stat(templateDir)
	if err != nil {
		return fmt.Errorf("访问模板目录失败: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("模板目录 %s 不是目录", templateDir)
	}
	if err := os.RemoveAll(workDir); err != nil {
		return fmt.Errorf("清理旧工作目录失败: %w", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("创建工作目录失败: %w", err)
	}

	return filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(templateDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if copyExcluded[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dest := filepath.Join(workDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			// 模板中的 symlink 一律跳过，避免把宿主路径带入生成产物。
			return nil
		}
		return copyFile(path, dest)
	})
}

func copyFile(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("复制文件失败: %w", err)
	}
	return nil
}
