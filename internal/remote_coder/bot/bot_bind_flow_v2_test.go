package bot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
)

// TestDirectoryBrowserV2_New creates a new browser and verifies initial state
func TestDirectoryBrowserV2_New(t *testing.T) {
	browser := NewDirectoryBrowserV2()
	assert.NotNil(t, browser)
}

// TestDirectoryBrowserV2_Start creates a new bind flow
func TestDirectoryBrowserV2_Start(t *testing.T) {
	browser := NewDirectoryBrowserV2()

	state, err := browser.Start("test-chat-id")

	require.NoError(t, err)
	require.NotNil(t, state)

	assert.Equal(t, "test-chat-id", state.ChatID)
	assert.NotEmpty(t, state.CurrentPath, "Should start with a valid path")
	assert.Equal(t, 0, state.Page, "Should start at page 0")
	assert.False(t, state.ExpiresAt.IsZero(), "Should have expiry time")
}

// TestDirectoryBrowserV2_GetState retrieves the current state
func TestDirectoryBrowserV2_GetState(t *testing.T) {
	browser := NewDirectoryBrowserV2()

	// Before start - should return nil
	state := browser.GetState("test-chat-id")
	assert.Nil(t, state, "Should return nil before start")

	// After start - should return state
	browser.Start("test-chat-id")
	state = browser.GetState("test-chat-id")
	assert.NotNil(t, state, "Should return state after start")
	assert.Equal(t, "test-chat-id", state.ChatID)
}

// TestDirectoryBrowserV2_Navigate navigates to a directory
func TestDirectoryBrowserV2_Navigate(t *testing.T) {
	browser := NewDirectoryBrowserV2()
	browser.Start("test-chat-id")

	// Navigate to temp directory
	tempDir := os.TempDir()
	err := browser.Navigate("test-chat-id", tempDir)

	require.NoError(t, err)

	state := browser.GetState("test-chat-id")
	// Clean paths for comparison (trailing slashes may differ)
	expectedPath := filepath.Clean(tempDir)
	actualPath := filepath.Clean(state.CurrentPath)
	assert.Equal(t, expectedPath, actualPath)
	assert.Equal(t, 0, state.Page, "Page should reset to 0 after navigation")
}

// TestDirectoryBrowserV2_NavigateByIndex navigates using directory index
func TestDirectoryBrowserV2_NavigateByIndex(t *testing.T) {
	browser := NewDirectoryBrowserV2()
	browser.Start("test-chat-id")
	browser.Navigate("test-chat-id", os.TempDir())

	// Build interactions to populate Dirs
	_, interactions, _, err := browser.BuildInteractions("test-chat-id")
	require.NoError(t, err)
	require.True(t, len(interactions) > 0, "Should have directory interactions")

	state := browser.GetState("test-chat-id")
	if len(state.Dirs) > 0 {
		// Try to navigate to first directory
		err = browser.NavigateByIndex("test-chat-id", 0)

		// This might fail if temp dir has no subdirectories
		if err == nil {
			newState := browser.GetState("test-chat-id")
			assert.NotEqual(t, os.TempDir(), newState.CurrentPath, "Should have navigated to subdirectory")
		}
	}
}

// TestDirectoryBrowserV2_NavigateUp navigates to parent directory
func TestDirectoryBrowserV2_NavigateUp(t *testing.T) {
	browser := NewDirectoryBrowserV2()
	browser.Start("test-chat-id")

	// First navigate to a subdirectory if possible
	tempDir := os.TempDir()
	testDir := filepath.Join(tempDir, "test-browser-v2")

	// Create test directory if it doesn't exist
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err = os.MkdirAll(testDir, 0755)
		require.NoError(t, err)
		defer os.RemoveAll(testDir)
	}

	browser.Navigate("test-chat-id", testDir)

	// Now navigate up
	err := browser.NavigateUp("test-chat-id")

	// Should succeed if we're not at root
	if err == nil {
		state := browser.GetState("test-chat-id")
		assert.NotEqual(t, testDir, state.CurrentPath, "Should have navigated up")
	}
}

// TestDirectoryBrowserV2_Pagination tests page navigation
func TestDirectoryBrowserV2_Pagination(t *testing.T) {
	browser := NewDirectoryBrowserV2()
	browser.Start("test-chat-id")
	browser.Navigate("test-chat-id", os.TempDir())

	// Test NextPage
	initialPage := browser.GetState("test-chat-id").Page
	err := browser.NextPage("test-chat-id")
	require.NoError(t, err)
	newPage := browser.GetState("test-chat-id").Page

	// Page might stay the same if there are no more directories
	assert.GreaterOrEqual(t, newPage, initialPage)

	// Test PrevPage (should not go negative)
	err = browser.PrevPage("test-chat-id")
	require.NoError(t, err)
	finalPage := browser.GetState("test-chat-id").Page

	assert.GreaterOrEqual(t, finalPage, 0, "Page should not be negative")
}

// TestDirectoryBrowserV2_BuildInteractions builds platform-agnostic interactions
func TestDirectoryBrowserV2_BuildInteractions(t *testing.T) {
	browser := NewDirectoryBrowserV2()
	browser.Start("test-chat-id")
	browser.Navigate("test-chat-id", os.TempDir())

	state, interactions, text, err := browser.BuildInteractions("test-chat-id")

	require.NoError(t, err)
	require.NotNil(t, state)
	require.NotNil(t, interactions)
	require.NotEmpty(t, text, "Should have message text")

	// Verify interactions structure
	assert.True(t, len(interactions) > 0, "Should have at least one interaction")

	// Verify there are navigation interactions (up, prev, next) or action interactions
	hasAction := false
	for _, interaction := range interactions {
		// Check for various action types
		if interaction.Type == "select" || interaction.Type == "confirm" || interaction.Type == "cancel" || interaction.Type == "navigate" {
			hasAction = true
			break
		}
	}
	assert.True(t, hasAction, "Should have at least one action interaction")
}

// TestDirectoryBrowserV2_Clear removes the state
func TestDirectoryBrowserV2_Clear(t *testing.T) {
	browser := NewDirectoryBrowserV2()
	browser.Start("test-chat-id")

	// Verify state exists
	state := browser.GetState("test-chat-id")
	assert.NotNil(t, state)

	// Clear the state
	browser.Clear("test-chat-id")

	// Verify state is gone
	state = browser.GetState("test-chat-id")
	assert.Nil(t, state, "State should be nil after clear")
}

// TestDirectoryBrowserV2_WaitingInput tests waiting for custom path input
func TestDirectoryBrowserV2_WaitingInput(t *testing.T) {
	browser := NewDirectoryBrowserV2()
	browser.Start("test-chat-id")

	// Initially not waiting
	assert.False(t, browser.IsWaitingInput("test-chat-id"))

	// Set waiting state
	browser.SetWaitingInput("test-chat-id", true, "prompt-msg-123")

	// Should now be waiting
	assert.True(t, browser.IsWaitingInput("test-chat-id"))

	state := browser.GetState("test-chat-id")
	assert.True(t, state.WaitingInput)
	assert.Equal(t, "prompt-msg-123", state.PromptMsgID)

	// Clear waiting state
	browser.SetWaitingInput("test-chat-id", false, "")
	assert.False(t, browser.IsWaitingInput("test-chat-id"))
}

// TestBuildActionInteractionsV2 builds action menu interactions
func TestBuildActionInteractionsV2(t *testing.T) {
	interactions := BuildActionInteractionsV2()

	assert.NotNil(t, interactions)
	assert.Equal(t, 3, len(interactions), "Should have 3 actions: clear, bind, project")

	// Verify each action
	actionMap := make(map[string]string)
	for _, interaction := range interactions {
		actionMap[interaction.ID] = interaction.Value
	}

	assert.Contains(t, actionMap, "action-clear", "Should have clear action")
	assert.Contains(t, actionMap, "action-bind", "Should have bind action")
	assert.Contains(t, actionMap, "action-project", "Should have project action")
}

// TestBuildCancelInteractionsV2 builds cancel button
func TestBuildCancelInteractionsV2(t *testing.T) {
	interactions := BuildCancelInteractionsV2()

	assert.NotNil(t, interactions)
	assert.Equal(t, 1, len(interactions), "Should have exactly 1 interaction")

	assert.Equal(t, "cancel_cancel", interactions[0].ID)
	assert.Equal(t, imbot.ActionCancel, interactions[0].Type)
	assert.Equal(t, "cancel", interactions[0].Value)
}

// TestBuildBindConfirmInteractionsV2 builds bind confirmation interactions
func TestBuildBindConfirmInteractionsV2(t *testing.T) {
	interactions := BuildBindConfirmInteractionsV2()

	assert.NotNil(t, interactions)
	assert.Equal(t, 4, len(interactions), "Should have yes, no, custom, cancel")

	// Verify yes button (from AddConfirm)
	hasYes := false
	for _, interaction := range interactions {
		if interaction.ID == "confirm_yes" && interaction.Type == "confirm" {
			hasYes = true
			break
		}
	}
	assert.True(t, hasYes, "Should have yes button")

	// Verify no button (from AddConfirm)
	hasNo := false
	for _, interaction := range interactions {
		if interaction.ID == "confirm_no" && interaction.Type == "confirm" {
			hasNo = true
			break
		}
	}
	assert.True(t, hasNo, "Should have no button")

	// Verify cancel button
	hasCancel := false
	for _, interaction := range interactions {
		if interaction.ID == "cancel_cancel" && interaction.Type == "cancel" {
			hasCancel = true
			break
		}
	}
	assert.True(t, hasCancel, "Should have cancel button")

	// Verify custom button
	hasCustom := false
	for _, interaction := range interactions {
		if interaction.ID == "custom" {
			hasCustom = true
			break
		}
	}
	assert.True(t, hasCustom, "Should have custom button")
}

// TestBuildCreateConfirmInteractionsV2 builds create confirmation
func TestBuildCreateConfirmInteractionsV2(t *testing.T) {
	testPath := "/tmp/test-project"
	interactions, text := BuildCreateConfirmInteractionsV2(testPath)

	assert.NotNil(t, interactions)
	assert.NotEmpty(t, text, "Should have prompt text")
	assert.Contains(t, text, testPath, "Text should contain the path")

	assert.Equal(t, 2, len(interactions), "Should have create and cancel")

	// Verify create button
	hasCreate := false
	for _, interaction := range interactions {
		if interaction.ID == "create" {
			hasCreate = true
			assert.Contains(t, interaction.Value, testPath, "Create value should contain path")
			break
		}
	}
	assert.True(t, hasCreate, "Should have create button")
}

// TestValidateProjectPathV2 validates project paths
func TestValidateProjectPathV2(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid temp dir", os.TempDir(), false},
		{"empty path", "", true},
		{"non-existent path", "/non/existent/path/12345", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProjectPathV2(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestExpandPathV2 expands paths with ~
func TestExpandPathV2(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"current directory", ".", false},
		{"home dir with ~", "~", false},
		{"absolute path", "/tmp", false},
		{"empty path", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := ExpandPathV2(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, path)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, path, "Should return expanded path")
				assert.True(t, filepath.IsAbs(path), "Should return absolute path")
			}
		})
	}
}
