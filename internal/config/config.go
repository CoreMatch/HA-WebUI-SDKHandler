// Package config 负责加载与校验 SDKHandler 的 YAML 配置。
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 是顶层配置结构。
type Config struct {
	Server          ServerConfig  `yaml:"server"`
	HRPAuth         HRPAuthConfig `yaml:"hrpauth"`
	Build           BuildConfig   `yaml:"build"`
	StateDir        string        `yaml:"state_dir"`
	ServeArtifacts  bool          `yaml:"serve_artifacts"`
	configPath      string        // 配置文件绝对路径（用于解析相对路径）
}

// ServerConfig 是 HTTP 服务配置。
type ServerConfig struct {
	ListenAddr string `yaml:"listen_addr"`
}

// HRPAuthConfig 是 HRPAuth 主服务连接配置。
type HRPAuthConfig struct {
	BaseURL            string `yaml:"base_url"`
	OpsToken           string `yaml:"ops_token"`
	PollIntervalSeconds int   `yaml:"poll_interval_seconds"`
}

// BuildConfig 是构建与发布管线配置。
type BuildConfig struct {
	TemplateDir          string   `yaml:"template_dir"`
	WorkDir              string   `yaml:"work_dir"`
	ArtifactsDir         string   `yaml:"artifacts_dir"`
	CurrentLink          string   `yaml:"current_link"`
	NPMCommand           string   `yaml:"npm_command"`
	IgnoreScriptsDefault bool     `yaml:"ignore_scripts_default"`
	NPMBlacklist         []string `yaml:"npm_blacklist"`
	BuildTimeout         int      `yaml:"build_timeout_seconds"` // 单次构建整体超时（秒），0 表示默认 20 分钟
}

// Load 读取并解析配置文件，随后应用默认值并校验必填项。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	cfg.configPath = path

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	// 相对路径统一基于配置文件所在目录解析，避免受启动 cwd 影响。
	cfg.resolveRelativePaths()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = ":8200"
	}
	if c.HRPAuth.PollIntervalSeconds <= 0 {
		c.HRPAuth.PollIntervalSeconds = 60
	}
	if c.Build.WorkDir == "" {
		c.Build.WorkDir = "./frontend-build"
	}
	if c.Build.ArtifactsDir == "" {
		c.Build.ArtifactsDir = "./artifacts"
	}
	if c.Build.CurrentLink == "" {
		c.Build.CurrentLink = "./current"
	}
	if c.Build.NPMCommand == "" {
		c.Build.NPMCommand = "npm"
	}
	if c.Build.BuildTimeout <= 0 {
		c.Build.BuildTimeout = 20 * 60
	}
	if c.StateDir == "" {
		c.StateDir = "./state"
	}
}

func (c *Config) validate() error {
	var missing []string
	if strings.TrimSpace(c.HRPAuth.BaseURL) == "" {
		missing = append(missing, "hrpauth.base_url")
	}
	if strings.TrimSpace(c.HRPAuth.OpsToken) == "" {
		missing = append(missing, "hrpauth.ops_token")
	}
	if strings.TrimSpace(c.Build.TemplateDir) == "" {
		missing = append(missing, "build.template_dir")
	}
	if len(missing) > 0 {
		return fmt.Errorf("配置缺少必填项: %s", strings.Join(missing, ", "))
	}
	if !c.Build.IgnoreScriptsDefault {
		// 允许 operator 显式放开；契约默认 true，此处仅做提示，不强制。
	}
	return nil
}

// resolveRelativePaths 把相对路径基于配置文件所在目录补全为绝对路径。
func (c *Config) resolveRelativePaths() {
	base := dirOf(c.configPath)
	if base == "" {
		return
	}
	c.Build.TemplateDir = absIfRelative(base, c.Build.TemplateDir)
	c.Build.WorkDir = absIfRelative(base, c.Build.WorkDir)
	c.Build.ArtifactsDir = absIfRelative(base, c.Build.ArtifactsDir)
	c.Build.CurrentLink = absIfRelative(base, c.Build.CurrentLink)
	c.StateDir = absIfRelative(base, c.StateDir)
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

func absIfRelative(base, p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return base + "/" + strings.TrimPrefix(p, "./")
}
