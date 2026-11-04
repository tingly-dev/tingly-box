package core

import "testing"

func TestParseSlashCommand(t *testing.T) {
	tests := []struct {
		in         string
		name, args string
		ok         bool
	}{
		{"/dir", "dir", "", true},
		{"/DIR 2", "dir", "2", true},
		{"  /dir  ", "dir", "", true},
		{" /dir", "dir", "", true},
		{"\u200b/dir", "dir", "", true},
		{"/dir\t2", "dir", "2", true},
		{"/dir\n2", "dir", "2", true},
		{"/cd  ~/my project ", "cd", "~/my project", true},
		{"/dir@tingly_bot", "dir", "", true},
		{"/Dir@Tingly_Bot 2", "dir", "2", true},
		{"@tingly_bot /dir", "dir", "", true},
		{"@tingly_bot  /cd /tmp", "cd", "/tmp", true},
		{"<@U123ABC> /dir 1", "dir", "1", true},
		{"<@!123456> /dir", "dir", "", true},
		{"<@U123|tingly> /dir", "dir", "", true},
		{"@_user_1 /dir", "dir", "", true},
		{"@_user_1 @_user_2 /status", "status", "", true},
		{"／dir", "dir", "", true},
		{"／cd /tmp", "cd", "/tmp", true},
		{"@tingly_bot/dir", "dir", "", true},
		{"@tingly_bot/cd /tmp", "cd", "/tmp", true},
		{"@tingly_bot\u00a0/dir", "dir", "", true},
		{"@\u673a\u5668\u4eba\u3000/dir 1", "dir", "1", true},
		{"@\u673a\u5668\u4eba/dir", "dir", "", true},
		{"<@U123>/dir", "dir", "", true},
		{"<@U123><@U456>/dir", "dir", "", true},
		{"@_user_1/dir", "dir", "", true},
		{"@a@b /dir", "dir", "", true},
		{"@tingly_bot\t/dir", "dir", "", true},
		{"\t \n@tingly_bot   /dir  \n", "dir", "", true},
		{"@tingly_bot\uff0fdir", "dir", "", true},
		{"/dir   path", "dir", "path", true},
		{"/dir \t path with  spaces  ", "dir", "path with  spaces", true},
		{"/dir\u3000path", "dir", "path", true},
		{"/dir@tingly_bot    path", "dir", "path", true},
		{"@tingly_bot   /cd   ~/my  project   ", "cd", "~/my  project", true},
		{"@cc/dir", "", "", false},
		{"@tb/dir", "", "", false},
		{"@tingly_bot/", "", "", true},
		{"/", "", "", true},
		{"hello", "", "", false},
		{"", "", "", false},
		{"@tingly_bot hello", "", "", false},
		{"@tingly_bot", "", "", false},
		{"@_user_1 hello /dir", "", "", false},
		{"<@U123> hello", "", "", false},
		{"@cc /dir", "", "", false},
		{"@tb /dir", "", "", false},
		{"see /dir", "", "", false},
	}
	for _, tt := range tests {
		got, ok := ParseSlashCommand(tt.in)
		if ok != tt.ok || got.Name != tt.name || got.Args != tt.args {
			t.Errorf("ParseSlashCommand(%q) = %+v, %v; want name %q args %q ok %v", tt.in, got, ok, tt.name, tt.args, tt.ok)
		}
	}
}

func TestSlashCommandText(t *testing.T) {
	if got := (SlashCommand{Name: "dir", Args: "2"}).Text(); got != "/dir 2" {
		t.Errorf("got %q", got)
	}
	if got := (SlashCommand{Name: "dir"}).Text(); got != "/dir" {
		t.Errorf("got %q", got)
	}
}
