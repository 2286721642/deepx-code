// Package config 负责 ~/.deepx/model.yaml 的读写。
//
// YAML 结构(每个 role 独立 base_url / model / api_key,允许用不同 provider):
//
//	flash:
//	  base_url: https://api.deepseek.com
//	  model: deepseek-v4-flash
//	  api_key: sk-...
//	pro:
//	  base_url: https://api.deepseek.com
//	  model: deepseek-v4-pro
//	  api_key: sk-...
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ModelEntry 单个 role(flash / pro)的完整配置。
type ModelEntry struct {
	BaseURL       string `yaml:"base_url"`
	Model         string `yaml:"model"`
	APIKey        string `yaml:"api_key"`
	ContextWindow int    `yaml:"context_window"` // 上下文窗口大小(tokens)
}

// WebConfig 本地 web dashboard 配置。
// Enabled 用指针:nil(yaml 里没写)= 采用默认值(开启);写了才按显式值。
type WebConfig struct {
	Enabled *bool `yaml:"enabled"` // nil => 默认开启
	Port    int   `yaml:"port"`    // 0 => 随机端口
}

// Config 整份 model.yaml 的反序列化目标。
type Config struct {
	Flash ModelEntry `yaml:"flash"`
	Pro   ModelEntry `yaml:"pro"`
	Web   WebConfig  `yaml:"web"`
}

// WebEnabled 解析 web dashboard 是否开启:env(DEEPX_WEB)优先于 config,config 没写默认开启。
// DEEPX_WEB 接受 off/0/false/no 关闭,on/1/true/yes 开启。
func (c *Config) WebEnabled() bool {
	if v, ok := os.LookupEnv("DEEPX_WEB"); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "off", "0", "false", "no":
			return false
		case "on", "1", "true", "yes":
			return true
		}
	}
	if c != nil && c.Web.Enabled != nil {
		return *c.Web.Enabled
	}
	return true
}

// WebPort 解析 web dashboard 端口:env(DEEPX_WEB_PORT)优先,其次 config,默认 0(随机)。
func (c *Config) WebPort() int {
	if v, ok := os.LookupEnv("DEEPX_WEB_PORT"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	if c != nil {
		return c.Web.Port
	}
	return 0
}

const (
	dirName  = ".deepx"
	fileName = "model.yaml"

	defaultBaseURL    = "https://api.deepseek.com"
	defaultFlashModel = "deepseek-v4-flash"
	defaultProModel   = "deepseek-v4-pro"
)

// Path 返回 ~/.deepx/model.yaml 绝对路径。
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户目录: %w", err)
	}
	return filepath.Join(home, dirName, fileName), nil
}

// Exists 配置文件是否已存在。出错或不存在均返回 false。
func Exists() bool {
	p, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Default 用单一 apiKey 构造初始配置:flash/pro 共享 base_url 和 key,只是 model id 不同。
// 用户之后可以手动编辑 model.yaml 把 flash 改成其它便宜模型 / 切换 base_url。
func Default(apiKey string) *Config {
	return &Config{
		Flash: ModelEntry{
			BaseURL:       defaultBaseURL,
			Model:         defaultFlashModel,
			APIKey:        apiKey,
			ContextWindow: defaultContextWindow(defaultFlashModel),
		},
		Pro: ModelEntry{
			BaseURL:       defaultBaseURL,
			Model:         defaultProModel,
			APIKey:        apiKey,
			ContextWindow: defaultContextWindow(defaultProModel),
		},
	}
}

// defaultContextWindow 根据模型名推断上下文窗口。含 deepseek 的模型默认 1M tokens。
func defaultContextWindow(model string) int {
	if strings.Contains(strings.ToLower(model), "deepseek") {
		return 1_048_576
	}
	return 65_536
}

// Load 从 ~/.deepx/model.yaml 读配置。文件缺失或解析失败返回 err。
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", p, err)
	}
	if c.Flash.ContextWindow <= 0 {
		c.Flash.ContextWindow = defaultContextWindow(c.Flash.Model)
	}
	if c.Pro.ContextWindow <= 0 {
		c.Pro.ContextWindow = defaultContextWindow(c.Pro.Model)
	}
	return &c, nil
}

// Save 写配置到 ~/.deepx/model.yaml,目录不存在会自动创建。
// 文件权限 0600(只有当前用户可读写,因为含 api key)。
func Save(c *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}
