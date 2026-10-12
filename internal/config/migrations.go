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
	// 从版本 "1" 迁移到 "2": ops_token -> client_id/client_secret
	{
		FromVersion: "1",
		ToVersion:   "2",
		Migrate: func(cfg map[string]interface{}) error {
			hrpauth, _ := cfg["hrpauth"].(map[string]interface{})
			if hrpauth != nil {
				// 如果有旧的 ops_token，我们无法自动映射到 client_id/secret，
				// 但我们可以保留结构，让用户手动填写。
				// 按照惯例，如果 ops_token 存在，可以提示用户它已弃用。
				delete(hrpauth, "ops_token")
				hrpauth["client_id"] = ""
				hrpauth["client_secret"] = ""
			}
			cfg["version"] = "2"
			return nil
		},
	},
	// 从版本 "2" 迁移到 "3": 添加 build.template_git_url
	{
		FromVersion: "2",
		ToVersion:   "3",
		Migrate: func(cfg map[string]interface{}) error {
			build, _ := cfg["build"].(map[string]interface{})
			if build != nil {
				if build["template_git_url"] == nil {
					build["template_git_url"] = "https://github.com/CoreMatch/HRPAuth-WebUI.git"
				}
			}
			cfg["version"] = "3"
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
