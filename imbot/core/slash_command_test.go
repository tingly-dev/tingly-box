package core

import "testing"

type parseCase struct {
	in         string
	name, args string
	ok         bool
}

func runParseCases(t *testing.T, group bool, cases []parseCase) {
	t.Helper()
	for _, tt := range cases {
		got, ok := ParseSlashCommand(tt.in, group)
		if ok != tt.ok || got.Name != tt.name || got.Args != tt.args {
			t.Errorf("ParseSlashCommand(%q, group=%v) = %+v, %v; want name %q args %q ok %v", tt.in, group, got, ok, tt.name, tt.args, tt.ok)
		}
	}
}

// plainCases parse the same in a group and in a direct chat.
var plainCases = []parseCase{
	{"/dir", "dir", "", true},
	{"/DIR 2", "dir", "2", true},
	{"  /dir  ", "dir", "", true},
	{" /dir", "dir", "", true},
	{"\u200b/dir", "dir", "", true},
	{"\u00a0/dir", "dir", "", true},
	{"\t \n/dir  \n", "dir", "", true},
	{"/dir\t2", "dir", "2", true},
	{"/dir\n2", "dir", "2", true},
	{"/dir   path", "dir", "path", true},
	{"/dir \t path with  spaces  ", "dir", "path with  spaces", true},
	{"/dir\u3000path", "dir", "path", true},
	{"/cd  ~/my project ", "cd", "~/my project", true},
	{"／dir", "dir", "", true},
	{"／cd /tmp", "cd", "/tmp", true},
	{"/", "", "", true},
	{"hello", "", "", false},
	{"", "", "", false},
	{"   ", "", "", false},
	{"see /dir", "", "", false},
	{"@cc /dir", "", "", false},
	{"@tb/dir", "", "", false},
}

func TestParseSlashCommandPlain(t *testing.T) {
	runParseCases(t, true, plainCases)
	runParseCases(t, false, plainCases)
}

// A group is where people @mention the bot: Telegram's /dir@bot, Slack and
// Discord's <@U1>, Feishu's @_user_1 and a plain @name (WeCom, WhatsApp),
// with or without a space before the command.
func TestParseSlashCommandGroup(t *testing.T) {
	runParseCases(t, true, []parseCase{
		{"/dir@tingly_bot", "dir", "", true},
		{"/Dir@Tingly_Bot 2", "dir", "2", true},
		{"/dir@tingly_bot    path", "dir", "path", true},
		{"@tingly_bot /dir", "dir", "", true},
		{"@tingly_bot  /cd /tmp", "cd", "/tmp", true},
		{"@tingly_bot/dir", "dir", "", true},
		{"@tingly_bot/cd /tmp", "cd", "/tmp", true},
		{"@tingly_bot\u00a0/dir", "dir", "", true},
		{"@tingly_bot\t/dir", "dir", "", true},
		{"@机器人\u3000/dir 1", "dir", "1", true},
		{"@机器人/dir", "dir", "", true},
		{"@tingly_bot／dir", "dir", "", true},
		{"\t \n@tingly_bot   /dir  \n", "dir", "", true},
		{"@tingly_bot   /cd   ~/my  project   ", "cd", "~/my  project", true},
		{"<@U123ABC> /dir 1", "dir", "1", true},
		{"<@!123456> /dir", "dir", "", true},
		{"<@U123|tingly> /dir", "dir", "", true},
		{"<@U123>/dir", "dir", "", true},
		{"<@U123><@U456>/dir", "dir", "", true},
		{"@_user_1 /dir", "dir", "", true},
		{"@_user_1/dir", "dir", "", true},
		{"@_user_1 @_user_2 /status", "status", "", true},
		{"@a@b /dir", "dir", "", true},
		{"@tingly_bot/", "", "", true},
		{"@tingly_bot", "", "", false},
		{"@tingly_bot hello", "", "", false},
		{"@_user_1 hello /dir", "", "", false},
		{"<@U123> hello", "", "", false},
		{"@cc/dir", "", "", false},
	})
}

// A direct chat has one party, so nothing there needs an @: only a plain
// "/name args" is a command, and the decorated forms are ordinary text.
func TestParseSlashCommandDirect(t *testing.T) {
	runParseCases(t, false, []parseCase{
		{"@tingly_bot /dir", "", "", false},
		{"@tingly_bot/dir", "", "", false},
		{"<@U123> /dir", "", "", false},
		{"<@U123>/dir", "", "", false},
		{"@_user_1 /dir", "", "", false},
		{"@_user_1/dir", "", "", false},
		{"\t@tingly_bot   /dir  ", "", "", false},
		// The bot's name is not stripped from the command either.
		{"/dir@tingly_bot", "dir@tingly_bot", "", true},
		{"/dir@tingly_bot 2", "dir@tingly_bot", "2", true},
	})
}

func TestSlashCommandText(t *testing.T) {
	if got := (SlashCommand{Name: "dir", Args: "2"}).Text(); got != "/dir 2" {
		t.Errorf("got %q", got)
	}
	if got := (SlashCommand{Name: "dir"}).Text(); got != "/dir" {
		t.Errorf("got %q", got)
	}
}
