// Package publish 负责把构建成功的 dist 产物原子发布到 artifacts/<build-id>/dist，
// 并原子切换 current 指针。
//
// 发布策略：
//  1. 把 work_dir/dist 复制到 artifacts/<build-id>/dist（带版本号，永不覆盖历史）。
//  2. 切换 current 指针：优先 symlink + 临时名 + rename 的原子替换；
//     不支持 symlink 的环境（如 Windows 无特权）退化为目录复制 + 哨兵文件。
//  3. 切换前绝不触碰现有 current；失败时保留上一版 current 继续服务。
package publish

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"ha-webui-sdkhandler/internal/state"
)

// Options 是发布输入。
type Options struct {
	ArtifactsDir string // artifacts 根目录
	CurrentLink  string // current 指针路径
	WorkDir      string // 构建工作目录（含 dist/）
	BuildID      string // 本次 build-id
	Record       state.BuildRecord // 待写入状态记录（build_id、sdk 清单等已填）
	Store        *state.Store
}

// Result 描述发布结果。
type Result struct {
	ArtifactDir string `json:"artifact_dir"` // artifacts/<build-id>/dist
	Current     string `json:"current"`      // 解析后的 current 目标（真实目录）
	Switched    bool   `json:"switched"`
}

// Publish 执行发布。任一步失败返回错误，且不切换 current。
func Publish(opts Options) (Result, error) {
	res := Result{}
	if err := os.MkdirAll(opts.ArtifactsDir, 0o755); err != nil {
		return res, fmt.Errorf("创建 artifacts 目录失败: %w", err)
	}
	artifactDist := filepath.Join(opts.ArtifactsDir, opts.BuildID, "dist")
	if err := copyDir(filepath.Join(opts.WorkDir, "dist"), artifactDist); err != nil {
		return res, fmt.Errorf("复制 dist 到 artifacts 失败: %w", err)
	}

	// 切换 current（原子；失败不改变现状）。
	if err := switchCurrent(opts.ArtifactsDir, opts.CurrentLink, artifactDist); err != nil {
		return res, fmt.Errorf("切换 current 失败（已保留上一版）: %w", err)
	}
	res.ArtifactDir = artifactDist
	res.Current = artifactDist
	res.Switched = true

	// 记录状态（成功后）。
	now := time.Now()
	rec := opts.Record
	rec.FinishedAt = &now
	rec.ArtifactDir = artifactDist
	if err := opts.Store.Record(rec); err != nil {
		// 状态记录失败不撤销已切换的 current，仅向上报错。
		return res, fmt.Errorf("写入构建状态失败: %w", err)
	}
	return res, nil
}

// switchCurrent 原子切换 current 指针。
// Linux/macOS 用 symlink：先写 临时 symlink 再 rename 覆盖，保证任意时刻
// current 要么指向旧版要么指向新版。
// Windows 或 symlink 不可用时退化为：复制 artifactDist 到 current.new，
// 写哨兵文件后 rename 覆盖。
func switchCurrent(artifactsDir, currentLink, artifactDist string) error {
	if canSymlink() {
		return switchBySymlink(currentLink, artifactDist)
	}
	return switchByCopy(currentLink, artifactDist)
}

// switchBySymlink 用 symlink 原子替换 current。
func switchBySymlink(currentLink, artifactDist string) error {
	parent := filepath.Dir(currentLink)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp := currentLink + ".tmp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	_ = os.Remove(tmp) // 清理可能的残留
	if err := os.Symlink(artifactDist, tmp); err != nil {
		return fmt.Errorf("创建临时 symlink 失败: %w", err)
	}
	if err := os.Rename(tmp, currentLink); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("原子替换 current 失败: %w", err)
	}
	return nil
}

// switchByCopy 退化为复制 + 哨兵文件（先复制到临时目录，写哨兵，再 rename）。
func switchByCopy(currentLink, artifactDist string) error {
	parent := filepath.Dir(currentLink)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp := currentLink + ".new-" + fmt.Sprintf("%d", time.Now().UnixNano())
	_ = os.RemoveAll(tmp)
	if err := copyDir(artifactDist, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("复制产物到临时目录失败: %w", err)
	}
	// 哨兵文件：标识该目录为完整可用的发布产物。
	sentinel := filepath.Join(tmp, ".sdkhandler-published")
	if err := os.WriteFile(sentinel, []byte("ok\n"), 0o644); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("写入哨兵文件失败: %w", err)
	}
	if err := os.Rename(tmp, currentLink); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("原子替换 current 失败: %w", err)
	}
	return nil
}

// canSymlink 探测环境是否支持创建符号链接。
func canSymlink() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	probe := filepath.Join(os.TempDir(), fmt.Sprintf("sdkhandler-symlink-probe-%d", time.Now().UnixNano()))
	// 探测目标必须存在。
	target := filepath.Join(os.TempDir(), "sdkhandler-symlink-target")
	_ = os.WriteFile(target+".txt", []byte("x"), 0o644)
	if err := os.Symlink(target+".txt", probe); err != nil {
		return false
	}
	_ = os.Remove(probe)
	_ = os.Remove(target + ".txt")
	return true
}

// copyDir 递归复制目录。
func copyDir(src, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dest, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		if e.Type()&os.ModeSymlink != 0 {
			// 不复制 symlink，避免破坏产物独立性。
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// ResolveCurrent 返回 current 当前指向的真实目录（用于 /api/revision 与静态托管）。
// 返回空字符串表示尚无已发布产物。
func ResolveCurrent(currentLink string) (string, error) {
	info, err := os.Lstat(currentLink)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(currentLink)
		if err != nil {
			return "", err
		}
		return target, nil
	}
	// 退化形态：current 是真实目录（复制+哨兵）。
	return currentLink, nil
}

// RevisionInfo 描述当前生效产物版本信息。
type RevisionInfo struct {
	BuildID     string `json:"build_id"`
	PublishedAt string `json:"published_at"`
	ArtifactDir string `json:"artifact_dir"`
	Current     string `json:"current"`
	Exists      bool   `json:"exists"`
}

// LoadRevision 从状态记录 + current 解析当前生效版本。
func LoadRevision(store *state.Store, currentLink string) (RevisionInfo, error) {
	info := RevisionInfo{Current: currentLink}
	cur, err := ResolveCurrent(currentLink)
	if err != nil {
		return info, fmt.Errorf("解析 current 失败: %w", err)
	}
	info.Current = cur
	rec := store.LatestByStatus(state.StatusSuccess)
	if rec == nil {
		info.Exists = cur != ""
		return info, nil
	}
	info.BuildID = rec.BuildID
	info.ArtifactDir = rec.ArtifactDir
	if rec.FinishedAt != nil {
		info.PublishedAt = rec.FinishedAt.Format(time.RFC3339)
	}
	info.Exists = cur != ""
	return info, nil
}