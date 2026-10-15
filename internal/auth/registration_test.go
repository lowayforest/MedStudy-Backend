package auth

import (
	"strings"
	"testing"
)

// TestPrepareRegisterInput 验证注册输入会清理用户名、保留合法密码，并补上默认时区。
func TestPrepareRegisterInput(t *testing.T) {
	const password = "Abc!defg"
	prepared, err := PrepareRegisterInput(RegisterInput{
		Username: "  小明123  ",
		Password: password,
	})
	if err != nil {
		t.Fatalf("合法注册输入不应被拒绝：%v", err)
	}
	if prepared.Username != "小明123" {
		t.Fatalf("用户名 = %q，期望去除首尾空白", prepared.Username)
	}
	if prepared.Password != password {
		t.Fatal("合法密码应保持原样")
	}
	if prepared.Timezone != "Asia/Shanghai" {
		t.Fatalf("默认时区 = %q，期望 Asia/Shanghai", prepared.Timezone)
	}
	// 恰好达到 128 字节上限且满足字符要求的密码应被接受。
	maxPassword := strings.Repeat("a", 126) + "A!"
	if _, err := PrepareRegisterInput(RegisterInput{Username: "demo", Password: maxPassword}); err != nil {
		t.Fatalf("符合最高长度的密码不应被拒绝：%v", err)
	}
}

// TestPrepareRegisterInputRejectsInvalid 验证密码长度及可见 ASCII 字符范围，
// 并拒绝缺少大小写字母或英文标点的密码及无效的 IANA 时区。
func TestPrepareRegisterInputRejectsInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input RegisterInput
	}{
		{
			name:  "username_too_short_after_trim",
			input: RegisterInput{Username: "  小明  ", Password: "Abc!defg"},
		},
		{
			name:  "password_too_short",
			input: RegisterInput{Username: "demo", Password: "Abc!def"},
		},
		{
			name:  "password_over_128_bytes",
			input: RegisterInput{Username: "demo", Password: strings.Repeat("a", 127) + "A!"},
		},
		{
			name:  "password_contains_chinese",
			input: RegisterInput{Username: "demo", Password: "Abc!def汉"},
		},
		{
			name:  "password_contains_space",
			input: RegisterInput{Username: "demo", Password: " Abc!defg "},
		},
		{
			name:  "password_missing_uppercase",
			input: RegisterInput{Username: "demo", Password: "abcdefg!"},
		},
		{
			name:  "password_missing_lowercase",
			input: RegisterInput{Username: "demo", Password: "ABCDEFG!"},
		},
		{
			name:  "password_missing_ascii_symbol",
			input: RegisterInput{Username: "demo", Password: "Abcdefg1"},
		},
		{
			name:  "password_fullwidth_symbol_is_not_ascii",
			input: RegisterInput{Username: "demo", Password: "Abc!def！"},
		},
		{
			name:  "invalid_timezone",
			input: RegisterInput{Username: "demo", Password: "Abc!defg", Timezone: "Not/A_Real_Zone"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := PrepareRegisterInput(test.input); err == nil {
				t.Fatal("无效注册输入应该返回错误")
			}
		})
	}
}
