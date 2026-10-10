package folder

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateCreateInput 验证创建资料夹的名称和类型规则；合法输入返回 nil，非法输入返回约定错误。
// 这里只调用业务校验函数，不发送 HTTP 请求，也不写入数据库。
func TestValidateCreateInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		// 正常资料类型：中文名称配合 material，应通过校验。
		{
			name:  "material",
			input: CreateInput{Name: "生理学资料", Type: "material"},
		},
		// 正常试卷类型：exam 也是允许的类型，应通过校验。
		{
			name:  "exam",
			input: CreateInput{Name: "历年真题", Type: "exam"},
		},
		// 名称前后有空白：去掉空白后名称合法，应通过校验。
		{
			name:  "surrounding_whitespace",
			input: CreateInput{Name: " \t生理学资料\n ", Type: "material"},
		},
		// 空名称：应返回名称错误。
		{
			name:    "empty_name",
			input:   CreateInput{Name: "", Type: "material"},
			wantErr: ErrInvalidName,
		},
		// 只有空白的名称：去除空白后为空，也应返回名称错误。
		{
			name:    "blank_name",
			input:   CreateInput{Name: " \t\n ", Type: "material"},
			wantErr: ErrInvalidName,
		},
		// 上限边界：100 个汉字合法，长度不能按 UTF-8 字节数计算。
		{
			name:  "100_chinese_characters",
			input: CreateInput{Name: strings.Repeat("医", 100), Type: "material"},
		},
		// 超过上限：101 个汉字应返回名称错误。
		{
			name:    "101_chinese_characters",
			input:   CreateInput{Name: strings.Repeat("医", 101), Type: "material"},
			wantErr: ErrInvalidName,
		},
		// 未支持的类型：other 应返回类型错误。
		{
			name:    "unsupported_type",
			input:   CreateInput{Name: "资料", Type: "other"},
			wantErr: ErrInvalidType,
		},
		// 缺少类型：空字符串应返回类型错误。
		{
			name:    "missing_type",
			input:   CreateInput{Name: "资料", Type: ""},
			wantErr: ErrInvalidType,
		},
	}

	// 每个案例成为独立子测试；errors.Is 检查错误身份，不比较错误文字。
	// 未填写 wantErr 的案例，其零值是 nil，表示预期没有错误。
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateCreateInput(test.input)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("期望错误 %v，实际得到 %v", test.wantErr, err)
			}
		})
	}
}
