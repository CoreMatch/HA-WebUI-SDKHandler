// Package state 负责构建历史状态的持久化与查询。
//
// 状态文件 state/builds.json 记录每次构建的结果（build-id、时间、
// SDK 清单版本指纹、成功/失败），供 HTTP API 查询最近构建状态；
// 发布阶段也依赖它记录当前生效版本。
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Status 描述一次构建的最终状态。
type Status string

const (
	StatusPending  Status = "pending"
	StatusBuilding Status = "building"
	StatusSuccess  Status = "success"
	StatusFailed   Status = "failed"
)

// BuildRecord 是一次构建的状态记录。
type BuildRecord struct {
	BuildID      string            `json:"build_id"`                 // 时间戳 + 短 hash
	Status       Status            `json:"status"`                   // pending/building/success/failed
	StartedAt    time.Time         `json:"started_at"`               // 构建开始时间
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`    // 构建结束时间（成功/失败后设置）
	SDKVersion   string            `json:"sdk_version,omitempty"`    // 本次构建对应的 SDK 列表摘要指纹
	SDKList      []string          `json:"sdk_list,omitempty"`       // 参与本次构建的 SDK 名称列表
	ManifestSums map[string]string `json:"manifest_sums,omitempty"`  // name -> sha256（包内容摘要）
	ArtifactDir  string            `json:"artifact_dir,omitempty"`   // 成功发布后的产物目录（相对 artifacts 根或绝对路径）
	Error        string            `json:"error,omitempty"`          // 失败原因摘要
}

// Store 线程安全地管理构建历史状态。
type Store struct {
	mu      sync.RWMutex
	path    string
	records []BuildRecord // 按时间倒序
}

// New 初始化状态仓库；目录不存在则创建，历史文件不存在则视为空。
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建状态目录失败: %w", err)
	}
	s := &Store{path: filepath.Join(dir, "builds.json")}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.records = nil
			return nil
		}
		return fmt.Errorf("读取状态文件失败: %w", err)
	}
	var records []BuildRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Errorf("解析状态文件失败: %w", err)
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].StartedAt.After(records[j].StartedAt)
	})
	s.records = records
	return nil
}

// persist 在持有写锁时落盘。
func (s *Store) persist() error {
	raw, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化状态失败: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("写入状态文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("替换状态文件失败: %w", err)
	}
	return nil
}

// Record 写入一条记录。仅保留最新的一条成功记录和当前正在进行/失败的记录，避免历史堆积。
func (s *Store) Record(rec BuildRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 查找是否存在同 ID 记录（通常是状态更新）
	found := false
	for i := range s.records {
		if s.records[i].BuildID == rec.BuildID {
			s.records[i] = rec
			found = true
			break
		}
	}
	if !found {
		s.records = append([]BuildRecord{rec}, s.records...)
	}

	// 严格限制记录数量：仅保留最近的 1 条记录，彻底消除历史堆积。
	if len(s.records) > 1 {
		s.records = s.records[:1]
	}

	return s.persist()
}

// Latest 返回最近一次构建记录；历史为空时返回 nil。
func (s *Store) Latest() *BuildRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.records) == 0 {
		return nil
	}
	cp := s.records[0]
	return &cp
}

// LatestByStatus 返回最近一次处于指定状态的记录。
func (s *Store) LatestByStatus(st Status) *BuildRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.records {
		if s.records[i].Status == st {
			cp := s.records[i]
			return &cp
		}
	}
	return nil
}

// List 返回全部记录副本（按时间倒序）。
func (s *Store) List() []BuildRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]BuildRecord, len(s.records))
	copy(out, s.records)
	return out
}
