package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Subagent sessions carry parent_session_id (2.1.258); it must survive the
// parse → fix → format round trip and stay absent when the client did not
// send it.
func TestMetadataUserID_ParentSessionIDRoundTrip(t *testing.T) {
	raw := `{"device_id":"d","account_uuid":"a","session_id":"s","parent_session_id":"p"}`
	m := ParseMetadataUserID(raw)
	require.NotNil(t, m)
	assert.Equal(t, "p", m.ParentSessionID)

	m.Fix(map[string]any{"device": "dev", "user_id": "acct"})
	assert.Equal(t, `{"device_id":"dev","account_uuid":"acct","session_id":"s","parent_session_id":"p"}`, m.Format())

	main := ParseMetadataUserID(`{"device_id":"d","account_uuid":"a","session_id":"s"}`)
	require.NotNil(t, main)
	assert.Equal(t, `{"device_id":"d","account_uuid":"a","session_id":"s"}`, main.Format())
}
