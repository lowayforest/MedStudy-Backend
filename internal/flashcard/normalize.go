package flashcard

import "strings"

func normalizeFlashcardText(text string) string {
	lower := strings.ToLower(text)
	words := strings.Fields(lower)
	normlize := strings.Join(words, " ")

	return normlize
}
