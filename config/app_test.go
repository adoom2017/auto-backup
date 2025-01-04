package config

import (
	"auto-backup/log"
	"testing"
)

func TestApp(t *testing.T) {
	config, err := LoadConfig("../config_example.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	if config.Log.Level != log.INFO {
		t.Fatalf("配置文件中的日志级别错误: %v", config.Log.Level)
	}
}
