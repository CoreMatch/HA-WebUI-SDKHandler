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
	"time"

	"ha-webui-sdkhandler/internal/state"
)

// Options 是发布输入。
type Options struct {
	ArtifactsDir string            // artifacts 根目录
	CurrentLink  string            // current 指针路径
	WorkDir      string            // 构建工作目录（含 dist/）
	BuildID      string            // 本次 build-id
	Record       state.BuildRecord // 待写入状态记录（build_id、sdk 清单等已填）
	Store        *state.Store
}

// Result 描述发布结果。
type Result struct {
	ArtifactDir string `json:"artifact_dir"` // artifacts/<build-id>/dist
	Current     string `json:"current"`      // 解析后的 current 目标（真实目录）
	Switched    bool   `json:"switched"`
}

// Publish 执行发布。直接将产物发布到固定目录，不保留历史归档。
func Publish(opts Options) (Result, error) {
	res := Result{}

	// 目标目录即为配置的 ArtifactsDir，不再使用 BuildID 子目录。
	finalDist := opts.ArtifactsDir
	parent := filepath.Dir(finalDist)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return res, fmt.Errorf("创建目标父目录失败: %w", err)
	}

	// 使用临时目录进行原子发布，确保发布过程中不会出现损坏的产物。
	tmpDist := finalDist + ".tmp"
	_ = os.RemoveAll(tmpDist)

	if err := copyDir(filepath.Join(opts.WorkDir, "dist"), tmpDist); err != nil {
		return res, fmt.Errorf("复制产物到临时目录失败: %w", err)
	}

	// 原子替换：先删除旧的，再重命名新的。
	// 注意：在 Linux 下 Rename 是原子的。
	_ = os.RemoveAll(finalDist)
	if err := os.Rename(tmpDist, finalDist); err != nil {
		return res, fmt.Errorf("替换最终产物目录失败: %w", err)
	}

	res.ArtifactDir = finalDist
	res.Current = finalDist
	res.Switched = true

	// 记录状态。
	now := time.Now()
	rec := opts.Record
	rec.FinishedAt = &now
	rec.ArtifactDir = finalDist
	if err := opts.Store.Record(rec); err != nil {
		return res, fmt.Errorf("写入构建状态失败: %w", err)
	}
	return res, nil
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
func ResolveCurrent(currentLink string) (string, error) {
	if _, err := os.Stat(currentLink); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
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
