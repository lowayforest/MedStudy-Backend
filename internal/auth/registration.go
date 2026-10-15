package auth

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

type RegisterInput struct {
	Username string
	Password string
	Timezone string
}

// PrepareRegisterInput 清理并校验注册输入，返回可用于后续注册流程的数据
func PrepareRegisterInput(input RegisterInput) (RegisterInput, error) {
	input.Username = strings.TrimSpace(input.Username)
	if utf8.RuneCountInString(input.Username) < 3 || utf8.RuneCountInString(input.Username) > 32 {
		return RegisterInput{}, errors.New("用户名长度不符合要求")
	}

	if len(input.Password) < 8 || len(input.Password) > 128 {
		return RegisterInput{}, errors.New("用户密码长度不符合要求")
	}

	var hasUpper, hasLower, hasSymbol bool

	for _, ch := range input.Password {
		if ch < '!' || ch > '~' {
			return RegisterInput{}, errors.New("密码只能包含可见 ASCII 字符")
		}

		if (ch >= '!' && ch <= '/') || (ch >= ':' && ch <= '@') || (ch >= '[' && ch <= '`') || (ch >= '{' && ch <= '~') {
			hasSymbol = true
		} else if ch >= 'a' && ch <= 'z' {
			hasLower = true
		} else if ch >= 'A' && ch <= 'z' {
			hasUpper = true
		}

	}

	if !hasLower {
		return RegisterInput{}, errors.New("密码应包含英文小写")
	}

	if !hasSymbol {
		return RegisterInput{}, errors.New("密码应包含英文标点符号")
	}

	if !hasUpper {
		return RegisterInput{}, errors.New("密码应包含英文大写")
	}

	if input.Timezone == "" {
		input.Timezone = "Asia/Shanghai"
	}

	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return RegisterInput{}, errors.New("时区无效")
	}

	return input, nil
}
