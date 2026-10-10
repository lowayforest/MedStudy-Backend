package folder

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// CreateInput 是创建资料夹时需要的业务输入。
type CreateInput struct {
	Name string
	Type string
}

var (
	ErrInvalidName = errors.New("资料夹名称去除前后空白后必须为 1 到 100 个字符")
	ErrInvalidType = errors.New("资料夹类型必须为 material 或 exam")
)

// ValidateCreateInput 检查创建资料夹的输入是否符合业务规则。
// 校验成功返回 nil；校验失败返回对应错误。当前实现待完成。
func ValidateCreateInput(input CreateInput) error {
	// TODO 1：去除 input.Name 前后的空白，取得用于校验的名称。
	newInput := CreateInput{
		Name: strings.TrimSpace(input.Name),
		Type: input.Type,
	}
	// TODO 2：按 Unicode 字符计数，长度不在 1～100 时返回 ErrInvalidName。
	if utf8.RuneCountInString(newInput.Name) < 1 || utf8.RuneCountInString(newInput.Name) > 100 {
		return ErrInvalidName
	}
	// TODO 3：Type 既不是 material，也不是 exam 时返回 ErrInvalidType。
	if newInput.Type != "material" && newInput.Type != "exam" {
		return ErrInvalidType
	}
	// 提示：字符串处理用 strings.TrimSpace，字符计数用 utf8.RuneCountInString。
	// 在开始填写时按需导入 strings 和 unicode/utf8 包。
	return nil
}
