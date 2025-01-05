package config

import (
	"auto-backup/log"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// 完整的配置结构
type OneDrive struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Scope        string `yaml:"scope"`
	RedirectURI  string `yaml:"redirect_uri"`
	BasePath     string `yaml:"base_path"`
}

type Log struct {
	Path       string       `yaml:"path"`
	Level      log.LogLevel `yaml:"level"`
	MaxSize    int          `yaml:"max_size"`
	MaxBackups int          `yaml:"max_backups"`
	Compress   bool         `yaml:"compress"`
}

type Backup struct {
	RootDir         string `yaml:"root_dir"`
	OutputDir       string `yaml:"output_dir"`
	Password        string `yaml:"password"`
	ForceFullBackup bool   `yaml:"force_full_backup"`
	Cron            string `yaml:"cron"`
}

type Config struct {
	OneDrive OneDrive `yaml:"onedrive"`
	Log      Log      `yaml:"log"`
	Backup   Backup   `yaml:"backup"`
}

// 加载完整配置
func LoadConfig(path string) (*Config, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		return nil, err
	}

	config := &Config{}
	err = yaml.Unmarshal(yamlFile, config)
	if err != nil {
		fmt.Printf("解析配置文件失败: %v\n", err)
		return nil, err
	}

	return config, nil
}
