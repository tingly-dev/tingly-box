package markdown

import "unicode/utf8"

// UTF16Len returns the length of a string in UTF-16 code units.
//
// Telegram measures entity offsets and lengths in UTF-16 code units,
// not Go string bytes or runes. Characters outside the BMP (codepoint > 0xFFFF)
// take 2 UTF-16 code units (a surrogate pair); all others take 1.
//
// Examples:
//   - "Hello" → 5 (5 ASCII characters)
//   - "你好" → 2 (2 BMP characters)
//   - "👍" → 2 (1 emoji, non-BMP, surrogate pair)
//   - "Hello 👍" → 8 (5 + 1 space + 2 for emoji)
func UTF16Len(text string) int {
	count := 0
	for _, r := range text {
		if r > 0xFFFF {
			count += 2 // Non-BMP character (surrogate pair)
		} else {
			count += 1 // BMP character
		}
	}
	return count
}

// UTF16OffsetTable builds a mapping from byte position to UTF-16 offset.
//
// Returns a slice where result[i] is the UTF-16 offset corresponding to
// byte position i in the input string. The last element (result[len(text)])
// contains the total UTF-16 length.
//
// This is useful for converting Go string indices to Telegram entity offsets.
func UTF16OffsetTable(text string) []int {
	offsets := make([]int, len(text)+1)
	utf16Pos := 0
	bytePos := 0

	for _, r := range text {
		runeLen := utf8.RuneLen(r)
		// Set offset for all bytes of this rune
		for i := 0; i < runeLen; i++ {
			offsets[bytePos+i] = utf16Pos
		}
		bytePos += runeLen

		if r > 0xFFFF {
			utf16Pos += 2
		} else {
			utf16Pos += 1
		}
	}
	offsets[len(text)] = utf16Pos // Total length
	return offsets
}
