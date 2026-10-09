package inject

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// errIllegalPath 表示归档内含非法路径（..、绝对路径、symlink/hardlink）。
var errIllegalPath = errors.New("归档包含非法路径")

// sanitizeArchivePath 校验归档内条目路径：必须是包内相对路径，
// 不得含 .. 段、不得为绝对路径。通过后返回规范化相对路径。
func sanitizeArchivePath(entry string) (string, error) {
	if entry == "" {
		return "", fmt.Errorf("%w: 空路径", errIllegalPath)
	}
	if strings.HasPrefix(entry, "/") || strings.HasPrefix(entry, `\`) {
		return "", fmt.Errorf("%w: 绝对路径 %q", errIllegalPath, entry)
	}
	// 统一分隔符后检查 .. 段。
	clean := filepath.ToSlash(entry)
	if strings.HasPrefix(clean, "./") {
		clean = strings.TrimPrefix(clean, "./")
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: 路径含 .. 段 %q", errIllegalPath, entry)
		}
		if seg == "" {
			return "", fmt.Errorf("%w: 路径含空段 %q", errIllegalPath, entry)
		}
	}
	return clean, nil
}

// extractTarGz 解包 tar.gz 归档到 destDir，保持包内相对结构。
// 拒绝 symlink / hardlink / 非法路径；文件覆盖前再做一次落盘前校验
// （filepath.Join 后必须仍在 destDir 内）。
func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("读取 gzip 失败: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar 失败: %w", err)
		}
		rel, err := sanitizeArchivePath(hdr.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if !isWithin(destDir, target) {
			return fmt.Errorf("%w: 落盘路径越界 %q", errIllegalPath, hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if err := writeEntryFile(target, tr); err != nil {
				return err
			}
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("创建目录失败: %w", err)
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("%w: 拒绝 symlink/hardlink %q", errIllegalPath, hdr.Name)
		case tar.TypeXGlobalHeader, tar.TypeXHeader:
			// 扩展头，跳过。
			continue
		default:
			return fmt.Errorf("%w: 不支持的条目类型 %d: %q", errIllegalPath, hdr.Typeflag, hdr.Name)
		}
	}
	return nil
}

// extractZip 解包 zip 归档到 destDir，保持包内相对结构。
// 拒绝 symlink / 非法路径；同样做落盘前越界校验。
func extractZip(r io.ReaderAt, size int64, destDir string) error {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return fmt.Errorf("读取 zip 失败: %w", err)
	}
	for _, f := range zr.File {
		rel, err := sanitizeArchivePath(f.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if !isWithin(destDir, target) {
			return fmt.Errorf("%w: 落盘路径越界 %q", errIllegalPath, f.Name)
		}
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: 拒绝 symlink %q", errIllegalPath, f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("创建目录失败: %w", err)
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("打开 zip 条目失败: %w", err)
		}
		err = writeEntryFile(target, rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// writeEntryFile 写入单个文件条目（先建父目录，再写文件）。
func writeEntryFile(target string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("创建父目录失败: %w", err)
	}
	f, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}

// isWithin 判断 child 是否位于 parent 目录之内。
func isWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
