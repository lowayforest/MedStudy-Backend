package folder

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCreateInput(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{
			name:  "material",
			input: CreateInput{Name: "生理学资料", Type: "material"},
		},
		{
			name:  "exam",
			input: CreateInput{Name: "历年真题", Type: "exam"},
		},
		{
			name:  "surrounding_whitespace",
			input: CreateInput{Name: " \t生理学资料\n ", Type: "material"},
		},
		{
			name:    "empty_name",
			input:   CreateInput{Name: "", Type: "material"},
			wantErr: ErrInvalidName,
		},
		{
			name:    "blank_name",
			input:   CreateInput{Name: " \t\n ", Type: "material"},
			wantErr: ErrInvalidName,
		},
		{
			name:  "100_chinese_characters",
			input: CreateInput{Name: strings.Repeat("医", 100), Type: "material"},
		},
		{
			name:    "101_chinese_characters",
			input:   CreateInput{Name: strings.Repeat("医", 101), Type: "material"},
			wantErr: ErrInvalidName,
		},
		{
			name:    "unsupported_type",
			input:   CreateInput{Name: "资料", Type: "other"},
			wantErr: ErrInvalidType,
		},
		{
			name:    "missing_type",
			input:   CreateInput{Name: "资料", Type: ""},
			wantErr: ErrInvalidType,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateCreateInput(test.input)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("期望错误 %v，实际得到 %v", test.wantErr, err)
			}
		})
	}
}
