package projects

import (
	"testing"

	"github.com/stretchr/testify/assert"

	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/agent"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// Remote owns the session command set and Projects keeps a copy until it can
// import Remote; the two must list the same commands.
func TestSessionCommandsMatchRemote(t *testing.T) {
	remote, projects := agent.Commands(), projectsapp.ChatCommands()
	if assert.Len(t, projects, len(remote)) {
		for i, c := range remote {
			assert.Equal(t, c.Name, projects[i].Name)
			assert.Equal(t, c.Usage, projects[i].Usage, c.Name)
			assert.Equal(t, c.Help, projects[i].Help, c.Name)
			assert.Equal(t, c.Order, projects[i].Order, c.Name)
		}
	}
}

// Remote owns the approval vocabulary and Projects keeps a copy until it can
// import Remote; the two must answer every word the same way.
func TestApprovalVocabularyMatchesRemote(t *testing.T) {
	for _, choice := range []string{interaction.ChoiceAllow, interaction.ChoiceAllowThread, interaction.ChoiceDeny} {
		assert.Equal(t, interaction.ApprovalWords(choice), projectsapp.ApprovalWords(choice), choice)
		for _, w := range interaction.ApprovalWords(choice) {
			got, ok := projectsapp.ApprovalChoice(w)
			assert.True(t, ok && got == choice, "%q", w)
		}
	}
	for _, w := range []string{"0", "1", "2", "3", "maybe", ""} {
		_, remoteOK := interaction.ApprovalChoice(w)
		_, projectsOK := projectsapp.ApprovalChoice(w)
		assert.Equal(t, remoteOK, projectsOK, "%q", w)
	}
}
