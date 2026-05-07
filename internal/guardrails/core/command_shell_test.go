package core

import (
	"reflect"
	"testing"
)

func TestExtractShellCommandTextUsesShellKeys(t *testing.T) {
	t.Parallel()

	got, ok := ExtractShellCommandText("Bash", map[string]interface{}{
		"command":     "ls -la ~/.ssh",
		"raw_command": "should-not-win",
	})
	if !ok {
		t.Fatalf("expected shell command to be extracted")
	}
	if got != "ls -la ~/.ssh" {
		t.Fatalf("expected command field, got %q", got)
	}
}

func TestExtractShellCommandTextFallsBackToRawJSON(t *testing.T) {
	t.Parallel()

	got, ok := ExtractShellCommandText("Bash", map[string]interface{}{
		"_raw": `{}{"command":"cat ~/.ssh/config","description":"read ssh config"}`,
	})
	if !ok {
		t.Fatalf("expected raw fallback to recover command")
	}
	if got != "cat ~/.ssh/config" {
		t.Fatalf("unexpected recovered command: %q", got)
	}
}

func TestParseShellCommand(t *testing.T) {
	parsed := ParseShellCommand(`ls -la ~/.ssh | grep id_rsa > out.txt`)
	if parsed == nil {
		t.Fatalf("expected parsed shell command")
	}
	if len(parsed.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(parsed.Commands))
	}
	if parsed.Commands[0].Program != "ls" {
		t.Fatalf("expected first program ls, got %q", parsed.Commands[0].Program)
	}
	if parsed.Commands[1].Program != "grep" {
		t.Fatalf("expected second program grep, got %q", parsed.Commands[1].Program)
	}
	if len(parsed.Operators) != 1 || parsed.Operators[0] != "|" {
		t.Fatalf("expected pipeline operator, got %#v", parsed.Operators)
	}
	if len(parsed.Redirects) != 1 || parsed.Redirects[0].Target != "out.txt" {
		t.Fatalf("expected redirect target out.txt, got %#v", parsed.Redirects)
	}
}

func TestParseShellCommandHandlesQuotesAndOperators(t *testing.T) {
	t.Parallel()

	parsed := ParseShellCommand(`grep "ssh config" ~/.ssh/config && cat ~/.ssh/config`)
	if parsed == nil {
		t.Fatalf("expected parsed shell command")
	}
	if len(parsed.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(parsed.Commands))
	}
	if parsed.Commands[0].Program != "grep" {
		t.Fatalf("expected grep, got %q", parsed.Commands[0].Program)
	}
	wantArgs := []string{"ssh config", "~/.ssh/config"}
	if !reflect.DeepEqual(parsed.Commands[0].Args, wantArgs) {
		t.Fatalf("unexpected first command args: %#v", parsed.Commands[0].Args)
	}
	if len(parsed.Operators) != 1 || parsed.Operators[0] != "&&" {
		t.Fatalf("unexpected operators: %#v", parsed.Operators)
	}
}

func TestParseShellCommandHandlesRedirects(t *testing.T) {
	t.Parallel()

	parsed := ParseShellCommand(`cat ~/.ssh/config 2>> errors.log`)
	if parsed == nil {
		t.Fatalf("expected parsed shell command")
	}
	if len(parsed.Redirects) != 1 {
		t.Fatalf("expected one redirect, got %d", len(parsed.Redirects))
	}
	if parsed.Redirects[0].Op != "2>>" || parsed.Redirects[0].Target != "errors.log" {
		t.Fatalf("unexpected redirect: %#v", parsed.Redirects[0])
	}
}

func TestCommandAttachDerivedFieldsBuildsShellAndNormalizedViews(t *testing.T) {
	t.Parallel()

	cmd := &Command{
		Name: "bash",
		Arguments: map[string]interface{}{
			"command": "ls -la ~/.ssh | grep config",
		},
	}

	cmd.AttachDerivedFields()

	if cmd.Shell == nil {
		t.Fatalf("expected parsed shell view")
	}
	if cmd.Normalized == nil {
		t.Fatalf("expected normalized command view")
	}
	if cmd.Normalized.Kind != "shell" {
		t.Fatalf("expected shell kind, got %q", cmd.Normalized.Kind)
	}
	if !reflect.DeepEqual(cmd.Normalized.Actions, []string{"execute", "read"}) {
		t.Fatalf("unexpected normalized actions: %#v", cmd.Normalized.Actions)
	}
	if !reflect.DeepEqual(cmd.Normalized.Resources, []string{"~/.ssh"}) {
		t.Fatalf("unexpected normalized resources: %#v", cmd.Normalized.Resources)
	}
}

func TestIsInstallCommandPositiveCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  ShellSimpleCommand
	}{
		{name: "pip install", cmd: ShellSimpleCommand{Program: "pip", Args: []string{"install", "requests"}}},
		{name: "pip2 install", cmd: ShellSimpleCommand{Program: "pip2", Args: []string{"install", "requests"}}},
		{name: "pip3 mixed case", cmd: ShellSimpleCommand{Program: "PIP3", Args: []string{"INSTALL", "requests"}}},
		{name: "pipx install", cmd: ShellSimpleCommand{Program: "pipx", Args: []string{"install", "httpie"}}},
		{name: "python -m pip install", cmd: ShellSimpleCommand{Program: "python3", Args: []string{"-m", "pip", "install", "requests"}}},
		{name: "uv pip install", cmd: ShellSimpleCommand{Program: "uv", Args: []string{"pip", "install", "requests"}}},
		{name: "uv tool install", cmd: ShellSimpleCommand{Program: "uv", Args: []string{"tool", "install", "ruff"}}},
		{name: "npm install", cmd: ShellSimpleCommand{Program: "npm", Args: []string{"install", "left-pad"}}},
		{name: "npm install with flag", cmd: ShellSimpleCommand{Program: "npm", Args: []string{"install", "--save", "left-pad"}}},
		{name: "npm i", cmd: ShellSimpleCommand{Program: "npm", Args: []string{"i", "left-pad"}}},
		{name: "npm.cmd add", cmd: ShellSimpleCommand{Program: "npm.cmd", Args: []string{"add", "left-pad"}}},
		{name: "pnpm add", cmd: ShellSimpleCommand{Program: "pnpm", Args: []string{"add", "left-pad"}}},
		{name: "yarn install", cmd: ShellSimpleCommand{Program: "yarn", Args: []string{"install"}}},
		{name: "yarn global add", cmd: ShellSimpleCommand{Program: "yarn", Args: []string{"global", "add", "typescript"}}},
		{name: "cargo install", cmd: ShellSimpleCommand{Program: "cargo", Args: []string{"install", "ripgrep"}}},
		{name: "cargo install with debug flag", cmd: ShellSimpleCommand{Program: "cargo", Args: []string{"install", "--debug", "ripgrep"}}},
		{name: "cargo add", cmd: ShellSimpleCommand{Program: "cargo", Args: []string{"add", "serde"}}},
		{name: "go get", cmd: ShellSimpleCommand{Program: "go", Args: []string{"get", "example.com/mod"}}},
		{name: "dotnet add package", cmd: ShellSimpleCommand{Program: "dotnet", Args: []string{"add", "package", "Newtonsoft.Json"}}},
		{name: "dotnet tool install", cmd: ShellSimpleCommand{Program: "dotnet", Args: []string{"tool", "install", "fantomas"}}},
		{name: "nuget install", cmd: ShellSimpleCommand{Program: "nuget", Args: []string{"install", "Newtonsoft.Json"}}},
		{name: "paket add", cmd: ShellSimpleCommand{Program: "paket", Args: []string{"add", "nuget", "Newtonsoft.Json"}}},
		{name: "gem install", cmd: ShellSimpleCommand{Program: "gem", Args: []string{"install", "bundler"}}},
		{name: "bundle add", cmd: ShellSimpleCommand{Program: "bundle", Args: []string{"add", "rails"}}},
		{name: "bundler install", cmd: ShellSimpleCommand{Program: "bundler", Args: []string{"install"}}},
		{name: "mvn dependency get", cmd: ShellSimpleCommand{Program: "mvn", Args: []string{"dependency:get", "-Dartifact=a:b:c"}}},
		{name: "mvnw dependency resolve", cmd: ShellSimpleCommand{Program: "mvnw", Args: []string{"dependency:resolve"}}},
		{name: "code install extension", cmd: ShellSimpleCommand{Program: "code", Args: []string{"--install-extension", "ms-python.python"}}},
		{name: "openvsx install", cmd: ShellSimpleCommand{Program: "openvsx", Args: []string{"install", "ms-python.python"}}},
		{name: "ovsx install", cmd: ShellSimpleCommand{Program: "ovsx", Args: []string{"install", "ms-python.python"}}},
		{name: "git clone", cmd: ShellSimpleCommand{Program: "git", Args: []string{"clone", "https://example.com/repo.git"}}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !isInstallCommand(tt.cmd) {
				t.Fatalf("expected install command for %#v", tt.cmd)
			}
		})
	}
}

func TestIsInstallCommandNegativeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  ShellSimpleCommand
	}{
		{name: "empty program", cmd: ShellSimpleCommand{}},
		{name: "pip list", cmd: ShellSimpleCommand{Program: "pip", Args: []string{"list"}}},
		{name: "python -m pip without install", cmd: ShellSimpleCommand{Program: "python", Args: []string{"-m", "pip", "list"}}},
		{name: "uv sync", cmd: ShellSimpleCommand{Program: "uv", Args: []string{"sync"}}},
		{name: "npm test", cmd: ShellSimpleCommand{Program: "npm", Args: []string{"test"}}},
		{name: "pnpm run", cmd: ShellSimpleCommand{Program: "pnpm", Args: []string{"run", "build"}}},
		{name: "yarn global without add", cmd: ShellSimpleCommand{Program: "yarn", Args: []string{"global", "list"}}},
		{name: "cargo build", cmd: ShellSimpleCommand{Program: "cargo", Args: []string{"build"}}},
		{name: "go test", cmd: ShellSimpleCommand{Program: "go", Args: []string{"test", "./..."}}},
		{name: "dotnet build", cmd: ShellSimpleCommand{Program: "dotnet", Args: []string{"build"}}},
		{name: "nuget restore", cmd: ShellSimpleCommand{Program: "nuget", Args: []string{"restore"}}},
		{name: "bundle exec", cmd: ShellSimpleCommand{Program: "bundle", Args: []string{"exec", "rspec"}}},
		{name: "mvn test", cmd: ShellSimpleCommand{Program: "mvn", Args: []string{"test"}}},
		{name: "code list extensions", cmd: ShellSimpleCommand{Program: "code", Args: []string{"--list-extensions"}}},
		{name: "openvsx search", cmd: ShellSimpleCommand{Program: "openvsx", Args: []string{"search", "python"}}},
		{name: "git status", cmd: ShellSimpleCommand{Program: "git", Args: []string{"status"}}},
		{name: "curl fetch", cmd: ShellSimpleCommand{Program: "curl", Args: []string{"-O", "https://example.com/file"}}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if isInstallCommand(tt.cmd) {
				t.Fatalf("did not expect install command for %#v", tt.cmd)
			}
		})
	}
}

func TestFirstArgIn(t *testing.T) {
	t.Parallel()

	if !firstArgIn([]string{"install", "pkg"}, "install", "add") {
		t.Fatal("expected firstArgIn to match the first argument")
	}
	if firstArgIn([]string{"pkg", "install"}, "install", "add") {
		t.Fatal("did not expect firstArgIn to match a later argument")
	}
	if firstArgIn(nil, "install") {
		t.Fatal("did not expect firstArgIn to match empty args")
	}
}

func TestContainsArg(t *testing.T) {
	t.Parallel()

	if !containsArg([]string{"--foo", "--install-extension", "pkg"}, "--install-extension") {
		t.Fatal("expected containsArg to find a matching argument")
	}
	if containsArg([]string{"--foo", "--bar"}, "--install-extension") {
		t.Fatal("did not expect containsArg to match a missing argument")
	}
}

func TestHasArgSequence(t *testing.T) {
	t.Parallel()

	if !hasArgSequence([]string{"-m", "pip", "install", "requests"}, "-m", "pip", "install") {
		t.Fatal("expected hasArgSequence to match a contiguous sequence")
	}
	if hasArgSequence([]string{"-m", "pip", "list"}, "-m", "pip", "install") {
		t.Fatal("did not expect hasArgSequence to match a different sequence")
	}
	if hasArgSequence([]string{"pip", "install"}, "-m", "pip", "install") {
		t.Fatal("did not expect hasArgSequence to match a longer sequence than args")
	}
}

func TestCommandAttachDerivedFieldsMarksInstallCommands(t *testing.T) {
	t.Parallel()

	cmd := &Command{
		Name: "bash",
		Arguments: map[string]interface{}{
			"command": "python -m pip install requests1",
		},
	}

	cmd.AttachDerivedFields()

	if cmd.Normalized == nil {
		t.Fatalf("expected normalized command view")
	}
	if !reflect.DeepEqual(cmd.Normalized.Actions, []string{"execute", ActionInstall}) {
		t.Fatalf("unexpected normalized actions: %#v", cmd.Normalized.Actions)
	}
}
