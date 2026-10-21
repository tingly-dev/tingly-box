package project

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Answering in a chat (.design/remote-into-projects.md, "要补的能力" ①): a
// permission approval has buttons where the chat can show them; everything a
// thread asks is otherwise answered by replying in the chat. While a chat has
// a request waiting, its next message is the answer.

type askOption struct{ label, description string }

type askQuestion struct {
	question string
	multi    bool
	options  []askOption
}

// askItems reads the questions of an AskUserQuestion input; a request without
// any is the one question it was asked with (fallback).
func askItems(input map[string]any, fallback string) []askQuestion {
	var out []askQuestion
	qs, _ := input["questions"].([]any)
	for _, raw := range qs {
		m, _ := raw.(map[string]any)
		q := askQuestion{}
		q.question, _ = m["question"].(string)
		if q.question == "" {
			continue
		}
		q.multi, _ = m["multiSelect"].(bool)
		opts, _ := m["options"].([]any)
		for _, o := range opts {
			om, _ := o.(map[string]any)
			label, _ := om["label"].(string)
			if label == "" {
				continue
			}
			desc, _ := om["description"].(string)
			q.options = append(q.options, askOption{label: label, description: strings.TrimSpace(desc)})
		}
		out = append(out, q)
	}
	if len(out) == 0 && strings.TrimSpace(fallback) != "" {
		out = []askQuestion{{question: fallback}}
	}
	return out
}

// requestText is how a chat is asked a request: what the thread wants, and
// how to answer by replying.
func requestText(e db.ThreadEvent, p requestPayload) string {
	switch e.Kind {
	case db.EvApproval:
		return approvalBody(e, p) + "\nReply y to allow, always to allow it for this thread, n to deny, or say what to do instead."
	case db.EvAsk:
		qs := askItems(p.Input, e.Content)
		var lines []string
		for i, q := range qs {
			head := q.question
			if len(qs) > 1 {
				head = fmt.Sprintf("%d) %s", i+1, q.question)
			}
			lines = append(lines, head)
			for j, o := range q.options {
				line := fmt.Sprintf("  %d. %s", j+1, o.label)
				if o.description != "" {
					line += ": " + o.description
				}
				lines = append(lines, line)
			}
		}
		hint := "Reply with a number or your own answer."
		if len(qs) > 1 {
			hint = "Reply with one line per question, each a number or your own answer."
		}
		for _, q := range qs {
			if q.multi {
				hint += " Pick several with commas, like 1,3."
				break
			}
		}
		return strings.Join(append(lines, hint), "\n")
	}
	return e.Content
}

// approvalBody is what an approval asks for; with buttons it is the whole
// prompt.
func approvalBody(e db.ThreadEvent, p requestPayload) string {
	body := "Approve: " + e.Content
	if reason := strings.TrimSpace(p.Reason); reason != "" {
		body += "\n" + reason
	}
	return body
}

// approvalReply reads a chat's reply to an approval: y, always or n; any
// other words deny it and tell the thread what to do instead.
func approvalReply(text string) Answer {
	ans := Answer{Via: db.SourceIM}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "y", "yes", "ok", "allow", "approve", "是", "好", "允许", "同意":
		ans.Allow = true
	case "a", "always", "总是", "总是允许":
		ans.Allow, ans.Scope = true, "thread"
	case "n", "no", "deny", "reject", "否", "不", "拒绝":
	default:
		ans.Answer = strings.TrimSpace(text)
	}
	return ans
}

// askReply reads a chat's reply to a question: one line per question (or the
// whole reply for one question), each an option's number, several numbers
// for a multi-select, or the user's own words.
func askReply(qs []askQuestion, text string) Answer {
	ans := Answer{Allow: true, Via: db.SourceIM}
	text = strings.TrimSpace(text)
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	switch {
	case len(qs) == 1:
		ans.Answers = map[string]string{qs[0].question: pickOptions(qs[0], text)}
	case len(qs) > 1 && len(lines) == len(qs):
		ans.Answers = map[string]string{}
		for i, q := range qs {
			ans.Answers[q.question] = pickOptions(q, lines[i])
		}
	default:
		// Not one line per question: it all answers the first (settleAsk).
		ans.Answer = text
	}
	return ans
}

// pickOptions turns "2" (or "1,3" for a multi-select) into the options'
// labels; anything else is the answer as written.
func pickOptions(q askQuestion, s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '，' || r == '、' })
	if len(fields) == 0 || (len(fields) > 1 && !q.multi) {
		return s
	}
	labels := make([]string, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(q.options) {
			return s
		}
		labels = append(labels, q.options[n-1].label)
	}
	return strings.Join(labels, ", ")
}
