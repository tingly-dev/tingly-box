package core

import (
	"regexp"
	"strings"
	"unicode"
)

// SlashCommand is a chat message that is a slash command, as every platform
// sends it once the platform's decoration is taken off.
type SlashCommand struct {
	// Name is lower-case, without the slash and without Telegram's "@bot"
	// suffix; empty for a bare "/".
	Name string
	// Args is what follows the name, trimmed.
	Args string
}

// Text is the command in its plain "/name args" form.
func (c SlashCommand) Text() string {
	return strings.TrimSpace("/" + c.Name + " " + c.Args)
}

// agentWords are the old agent prefixes ("@cc fix it"); they are messages,
// never mentions to look past.
var agentWords = map[string]bool{"@cc": true, "@tb": true, "@mock": true}

// angleMention is a Slack or Discord mention: <@U123>, <@!123>, <@U123|name>.
var angleMention = regexp.MustCompile(`^<@[!&]?[^<>\s]+>`)

// feishuMention is Feishu/Lark's placeholder for a mentioned user.
var feishuMention = regexp.MustCompile(`^@_user_\d+`)

// ParseSlashCommand reads a slash command from a chat message. The same text
// arrives differently by platform and place: Telegram groups send
// "/dir@mybot", Slack and Discord "<@U1> /dir", Feishu "@_user_1 /dir",
// WeCom and WhatsApp "@mybot /dir", DingTalk " /dir" (a leading space), and
// Chinese keyboards type "／dir" (a full-width slash). ok is false for a
// message that is not a command after all of that is taken off.
func ParseSlashCommand(text string) (cmd SlashCommand, ok bool) {
	t := trimChat(text)
	for {
		next, stripped := stripMention(t)
		if !stripped {
			break
		}
		t = next
	}
	if t == "" {
		return SlashCommand{}, false
	}
	if t[0] != '/' {
		r := []rune(t)
		if r[0] != '\uff0f' {
			return SlashCommand{}, false
		}
		t = "/" + string(r[1:])
	}
	head, rest, _ := strings.Cut(t[1:], " ")
	if i := strings.IndexFunc(head, unicode.IsSpace); i >= 0 {
		head, rest = head[:i], head[i:]+" "+rest
	}
	name, _, _ := strings.Cut(head, "@")
	return SlashCommand{Name: strings.ToLower(name), Args: trimChat(rest)}, true
}

// stripMention takes one leading mention off text, if there is one before a
// slash command, with or without a space between them ("@bot /dir",
// "@bot/dir"); "@bot hello" stays as it is, and so do the agent words.
func stripMention(t string) (string, bool) {
	if t == "" {
		return t, false
	}
	var loc []int
	switch {
	case angleMention.MatchString(t):
		loc = angleMention.FindStringIndex(t)
	case feishuMention.MatchString(t):
		loc = feishuMention.FindStringIndex(t)
	case t[0] == '@':
		end := strings.IndexFunc(t[1:], func(r rune) bool {
			return unicode.IsSpace(r) || r == '/' || r == '\uff0f' || r == '@' || r == '<'
		})
		if end < 0 {
			return t, false
		}
		end++
		if agentWords[strings.ToLower(t[:end])] {
			return t, false
		}
		loc = []int{0, end}
	default:
		return t, false
	}
	rest := trimChat(t[loc[1]:])
	if rest == "" || (rest[0] != '/' && !strings.HasPrefix(rest, "\uff0f") && rest[0] != '@' && rest[0] != '<') {
		return t, false
	}
	return rest, true
}

// trimChat trims whitespace chat platforms add: ordinary spaces, no-break
// spaces and zero-width characters.
func trimChat(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff' || r == '\u2060'
	})
}
