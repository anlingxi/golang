package langdetect

import (
	"strings"
	"unicode"
)

const (
	ContentTypeZH    = "zh"
	ContentTypeEN    = "en"
	ContentTypeCode  = "code"
	ContentTypeMixed = "mixed"
)

// DetectContentType classifies text into zh/en/code/mixed for index and query routing.
// 这是如何实现语言检测的：
func DetectContentType(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ContentTypeEN
	}

	if looksLikeCode(trimmed) {
		return ContentTypeCode
	}

	var totalLetters int
	var zhLetters int

	// 如何进行的中文字符统计？
	for _, r := range trimmed {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			totalLetters++
		}
		if isCJK(r) {
			zhLetters++
		}
	}

	if totalLetters == 0 {
		return ContentTypeEN
	}

	ratio := float64(zhLetters) / float64(totalLetters)
	switch {
	case ratio >= 0.80:
		return ContentTypeZH
	case ratio >= 0.20:
		return ContentTypeMixed
	default:
		return ContentTypeEN
	}
}

func looksLikeCode(text string) bool {
	signals := 0
	keywords := []string{
		"func ", "package ", "import ", "return ", "class ", "def ",
		"public ", "private ", "const ", "var ", "let ", "if ", "for ", "while ",
	}
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			signals++
		}
	}

	if strings.Count(text, "{")+strings.Count(text, "}") >= 2 {
		signals++
	}
	if strings.Count(text, "(")+strings.Count(text, ")") >= 2 {
		signals++
	}
	if strings.Count(text, ";") >= 2 {
		signals++
	}
	if strings.Contains(text, "=>") || strings.Contains(text, "::") {
		signals++
	}

	return signals >= 3
}

func isCJK(r rune) bool {
	return r >= 0x4E00 && r <= 0x9FFF
}
