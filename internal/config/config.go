package config

import (
	"errors"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL      string
	DatabasePassword string
}

// 在这里定义 Config，并实现 LoadFile；具体步骤见当前学习任务。
func LoadFile(filename string) (Config, error) {
	values, err := godotenv.Read(filename)
	if err != nil {
		return Config{}, errors.New("读取配置文件失败")
	}

	cfg := Config{
		DatabaseURL:      strings.TrimSpace(values["DATABASE_URL"]),
		DatabasePassword: values["POSTGRES_PASSWORD"],
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("数据库连接地址不能为空")
	}

	if cfg.DatabasePassword == "" {
		return Config{}, errors.New("数据库密码不能为空")
	}
	return cfg, nil
}
