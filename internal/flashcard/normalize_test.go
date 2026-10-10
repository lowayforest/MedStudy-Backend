package flashcard

import "testing"

type testCase struct {
	name  string
	input string
	want  string
}

// TestNormalizeFlashcardText 验证卡片文本去除前后空白、合并连续空白及英文字母转小写。
// 这是文本处理单元测试，不涉及卡片入库或数据库去重约束。
func TestNormalizeFlashcardText(t *testing.T) {
	tests := []testCase{
		// 中英文混合：ECG 转为 ecg，制表符和多余空格合并，末尾换行去除。
		{
			name:  "mixed_text",
			input: "  ECG\t  检查\n",
			want:  "ecg 检查",
		},
		// 纯中文：只去掉前后空白，汉字内容保持不变。
		{
			name:  "chinese",
			input: "  心输出量  ",
			want:  "心输出量",
		},
		// 空字符串：处理后仍为空，不应产生额外内容。
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		// 只有空白：处理后应得到空字符串。
		{
			name:  "whitespace",
			input: " \t\n ",
			want:  "",
		},
	}

	// 对每个输入调用标准化函数，并逐一比较实际字符串和预期字符串。
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeFlashcardText(test.input)
			if got != test.want {
				t.Fatalf("期望 %q, 实际得到 %q", test.want, got)
			}
		})
	}
}
