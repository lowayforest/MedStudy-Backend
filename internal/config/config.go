package config

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL      string
	DatabasePassword string
	JWTSigningKey    []byte
}

// LoadFile 从 filename 指定的文件读取数据库配置和 Base64 编码的 JWT 签名密钥。
// 成功返回包含解码后密钥的 Config；文件缺失或配置无效时返回零值 Config 和错误。
// 读取过程不修改进程环境变量。
func LoadFile(filename string) (Config, error) {
	values, err := godotenv.Read(filename)
	if err != nil {
		return Config{}, errors.New("读取配置文件失败")
	}

	encoded := strings.TrimSpace(values["JWT_SIGNING_KEY_BASE64"])
	key, err := base64.StdEncoding.Strict().DecodeString(encoded)

	if err != nil {
		return Config{}, errors.New("JWT 签名密钥格式无效")
	}

	if len(key) < 32 {
		return Config{}, errors.New("JWT 签名密钥至少需要 32 字节")
	}

	cfg := Config{
		DatabaseURL:      strings.TrimSpace(values["DATABASE_URL"]),
		DatabasePassword: values["POSTGRES_PASSWORD"],
		JWTSigningKey:    key,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("数据库连接地址不能为空")
	}

	if cfg.DatabasePassword == "" {
		return Config{}, errors.New("数据库密码不能为空")
	}
	return cfg, nil
}
