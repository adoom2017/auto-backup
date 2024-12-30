package config

import (
	"auto-backup/utils/logger"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

var (
	GlobalConfig = &Config{}
)

func init() {
	// 设置默认值
	GlobalConfig = &Config{
		OneDrive: OneDrive{
			ClientID:     "",
			ClientSecret: "",
			RedirectURI:  "http://localhost:8080/token",
			Scope:        "files.readwrite offline_access",
		},
		Log: Log{
			Path:     "./logs",
			Level:    logger.LogDebug,
			BaseName: "app",
			MaxDays:  7,
		},
	}
}

// 完整的配置结构
type OneDrive struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Scope        string `yaml:"scope"`
	RedirectURI  string `yaml:"redirect_uri"`
}

type Log struct {
	Path     string `yaml:"path"`
	BaseName string `yaml:"base_name"`
	Level    int    `yaml:"level"`
	MaxDays  int    `yaml:"max_days"`
}

type Backup struct {
	RootDir         string `yaml:"root_dir"`
	OutputDir       string `yaml:"output_dir"`
	Password        string `yaml:"password"`
	ForceFullBackup bool   `yaml:"force_full_backup"`
}

type Config struct {
	OneDrive OneDrive `yaml:"onedrive"`
	Log      Log      `yaml:"log"`
	Backup   Backup   `yaml:"backup"`
}

// 加载完整配置
func LoadConfig(path string) error {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		return err
	}

	err = yaml.Unmarshal(yamlFile, GlobalConfig)
	if err != nil {
		fmt.Printf("解析配置文件失败: %v\n", err)
		return err
	}

	return nil
}
