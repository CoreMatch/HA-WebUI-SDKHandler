package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Migration 定义了配置迁移的结构。
type Migration struct {
	FromVersion string
	ToVersion   string
	Migrate     func(cfg map[string]interface{}) error
}

// migrations 存储所有已注册的迁移。
var migrations = []Migration{
	// 示例：从版本 "0" (未定义) 迁移到 "1"
	{
		FromVersion: "",
		ToVersion:   "1",
		Migrate: func(cfg map[string]interface{}) error {
			cfg["version"] = "1"
			return nil
		},
	},
}

// Migrate 执行配置迁移。
func Migrate(path string, yamlConfig map[string]interface{}) error {
	version, _ := yamlConfig["version"].(string)

	for {
		if version == ConfigVersion {
			break
		}

		found := false
		for _, m := range migrations {
			if m.FromVersion == version {
				fmt.Printf("执行迁移: %s -> %s\n", m.FromVersion, m.ToVersion)

				// 备份当前配置
				if err := Backup(path, version); err != nil {
					return fmt.Errorf("备份配置失败: %w", err)
				}

				// 执行迁移
				if err := m.Migrate(yamlConfig); err != nil {
					return fmt.Errorf("执行迁移 %s 到 %s 失败: %w", m.FromVersion, m.ToVersion, err)
				}

				version = m.ToVersion
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("找不到从版本 %s 升级的路径", version)
		}
	}

	// 保存迁移后的配置
	data, err := yaml.Marshal(yamlConfig)
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}
