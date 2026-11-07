package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestTemplateDirDefaultConsistency 钉住 template_dir 的两处默认值一致：
// CreateDefault 生成的示例配置与 applyDefaults 的空值回落。
// 两者曾经漂移（"./HRPAuth-WebUI" vs "../HRPAuth-WebUI"）："../" 会把模板仓
// 解析到配置文件所在目录的上级，容器部署时（配置在 /app）即根目录，非 root
// 用户不可写；统一为 "./HRPAuth-WebUI"（见 ede3a47 与 applyDefaults 注释）。
func TestTemplateDirDefaultConsistency(t *testing.T) {
	const want = "./HRPAuth-WebUI"

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := CreateDefault(path); err != nil {
		t.Fatalf("CreateDefault: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取生成的默认配置: %v", err)
	}
	var parsed struct {
		Build struct {
			TemplateDir string `yaml:"template_dir"`
		} `yaml:"build"`
	}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("解析生成的默认配置: %v", err)
	}
	if parsed.Build.TemplateDir != want {
		t.Fatalf("CreateDefault template_dir = %q, 期望 %q", parsed.Build.TemplateDir, want)
	}

	cfg := &Config{}
	cfg.applyDefaults()
	if cfg.Build.TemplateDir != want {
		t.Fatalf("applyDefaults template_dir = %q, 期望 %q", cfg.Build.TemplateDir, want)
	}
}
