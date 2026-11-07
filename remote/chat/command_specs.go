package chat

import (
	"slices"

	"github.com/tingly-dev/tingly-box/remote/agent"
)

// accessTier says who is offered a command in /help (and may run it).
type accessTier int

const (
	// tierAny is offered to every chat.
	tierAny accessTier = iota
	// tierPicker is offered to a direct chat (it may pick a registered folder
	// or project), and to anyone with the "Any folder and files" permission.
	tierPicker
	// tierPrivileged is offered only with that permission.
	tierPrivileged
)

// commandSpec is a command as /help and the platform menu list it. Remote's
// own commands are declared below; the session commands come from Projects
// (agent.Commands). Both feed commandSpecs, which is the one list
// the menu, /help and the session-command dispatch are built from.
type commandSpec struct {
	Name string
	// Usage is how it is typed.
	Usage string
	// PickerUsage is Usage for a chat that may only pick; empty means Usage.
	PickerUsage string
	// Help is the one description, used by the menu and by /help.
	Help  string
	Order int
	tier  accessTier
}

// ownCommands are the listed commands Remote runs itself: they need the chat
// platform's UI (keyboards, the directory browser) and the access policy.
// /help, /pair and /join are not listed.
var ownCommands = []commandSpec{
	{Name: "cd", Usage: "/cd <path>", Help: "Work in another folder", Order: 90, tier: tierPrivileged},
	{Name: "dir", Usage: "/dir [N|path]", PickerUsage: "/dir [N]", Help: "List registered folders, or switch to one", Order: 85, tier: tierPicker},
	{Name: "project", Usage: "/project [N]", Help: "List projects, or switch to a project's coordinator", Order: 84, tier: tierPicker},
}

// commandSpecs is every listed command, first listed first.
func commandSpecs() []commandSpec {
	specs := slices.Clone(ownCommands)
	for _, c := range agent.Commands() {
		specs = append(specs, commandSpec{Name: c.Name, Usage: c.Usage, Help: c.Help, Order: c.Order})
	}
	slices.SortStableFunc(specs, func(a, b commandSpec) int { return b.Order - a.Order })
	return specs
}

// specHelp is the description of a listed command.
func specHelp(name string) string {
	for _, c := range commandSpecs() {
		if c.Name == name {
			return c.Help
		}
	}
	panic("chat: no command spec for " + name)
}

// specOrder is the listing position of a command.
func specOrder(name string) int {
	for _, c := range commandSpecs() {
		if c.Name == name {
			return c.Order
		}
	}
	panic("chat: no command spec for " + name)
}

// usageFor is how /help shows the command to a chat; ok is false when it is
// not offered to it.
func (c commandSpec) usageFor(privileged, group bool) (usage string, ok bool) {
	switch c.tier {
	case tierPrivileged:
		return c.Usage, privileged
	case tierPicker:
		switch {
		case privileged:
			return c.Usage, true
		case !group:
			if c.PickerUsage != "" {
				return c.PickerUsage, true
			}
			return c.Usage, true
		}
		return "", false
	}
	return c.Usage, true
}

// isSessionCommand reports whether the agent runs the command.
func isSessionCommand(name string) bool { return agent.IsCommand(name) }
