package interaction

import (
	"slices"
	"strings"
)

// Values an approval is answered with, by button or by ApprovalChoice.
const (
	ChoiceAllow       = "allow"
	ChoiceAllowThread = "always"
	ChoiceDeny        = "deny"
)

// approvalWords are the words a human answers an approval with in chat: the one
// vocabulary for every host and agent. The first two words of a choice are the
// ones shown to users; the others are accepted. Numbers are not used: they
// pick an option of a question.
var approvalWords = []struct {
	choice string
	words  []string
}{
	{ChoiceAllow, []string{"y", "yes", "ok", "allow", "approve", "是", "好", "允许", "同意"}},
	{ChoiceAllowThread, []string{"a", "always", "总是", "总是允许"}},
	{ChoiceDeny, []string{"n", "no", "deny", "reject", "否", "不", "拒绝"}},
}

// ApprovalChoice reads a human's words as an approval answer: ChoiceAllow,
// ChoiceAllowThread or ChoiceDeny. ok is false for anything else.
func ApprovalChoice(text string) (choice string, ok bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, a := range approvalWords {
		if slices.Contains(a.words, text) {
			return a.choice, true
		}
	}
	return "", false
}

// ApprovalWords are the words that answer with the choice, the ones to show
// first.
func ApprovalWords(choice string) []string {
	for _, a := range approvalWords {
		if a.choice == choice {
			return slices.Clone(a.words)
		}
	}
	return nil
}
