// Package commands holds Remote's chat command aliases shared by access and dispatch.
package commands

// Alias pairs a spelling with the command it invokes.
type Alias struct {
	Name    string
	Command string
}

var aliases = [...]Alias{
	{"start", "help"},
	{"h", "help"},
	{"bind", "pair"},
	{"d", "dir"},
	{"p", "project"},
	{"s", "session"},
	{"n", "new"},
	{"cls", "clear"},
	{"stat", "status"},
	{"v", "verbose"},
}

// Canonical resolves an alias, or returns name unchanged.
func Canonical(name string) string {
	for _, alias := range aliases {
		if alias.Name == name {
			return alias.Command
		}
	}
	return name
}

// Aliases returns the spellings registered for a command.
func Aliases(name string) []string {
	var result []string
	for _, alias := range aliases {
		if alias.Command == name {
			result = append(result, alias.Name)
		}
	}
	return result
}
