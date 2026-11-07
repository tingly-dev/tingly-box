package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApprovalChoice(t *testing.T) {
	for choice, words := range map[string][]string{
		ChoiceAllow:       {"y", " YES ", "ok", "allow", "approve", "是", "同意"},
		ChoiceAllowThread: {"a", "Always", "总是"},
		ChoiceDeny:        {"n", "No", "deny", "reject", "拒绝"},
	} {
		for _, w := range words {
			got, ok := ApprovalChoice(w)
			assert.True(t, ok, w)
			assert.Equal(t, choice, got, w)
		}
	}
	for _, w := range []string{"1", "2", "3", "0", "maybe", "use staging", ""} {
		_, ok := ApprovalChoice(w)
		assert.False(t, ok, "%q is not an approval word (numbers pick an option of a question)", w)
	}
}

// Every listed word reads back as its own choice, so the shown words and the
// accepted ones cannot drift apart.
func TestApprovalWordsReadBack(t *testing.T) {
	for _, choice := range []string{ChoiceAllow, ChoiceAllowThread, ChoiceDeny} {
		words := ApprovalWords(choice)
		assert.GreaterOrEqual(t, len(words), 2, choice)
		for _, w := range words {
			got, ok := ApprovalChoice(w)
			assert.True(t, ok && got == choice, "%q reads as %q", w, choice)
		}
	}
}
