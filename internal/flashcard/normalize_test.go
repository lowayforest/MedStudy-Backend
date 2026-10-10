package flashcard

import "testing"

type testCase struct {
	name  string
	input string
	want  string
}

func TestNormalizeFlashcardText(t *testing.T) {
	tests := []testCase{
		{
			name:  "mixed_text",
			input: "  ECG\t  检查\n",
			want:  "ecg 检查",
		},
		{
			name:  "chinese",
			input: "  心输出量  ",
			want:  "心输出量",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace",
			input: " \t\n ",
			want:  "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeFlashcardText(test.input)
			if got != test.want {
				t.Fatalf("期望 %q, 实际得到 %q", test.want, got)
			}
		})
	}
}
