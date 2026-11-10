package tools

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWebBackendDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		tool      string
		env       map[string]string
		available bool
		want      string
		fail      bool
	}{
		{BuiltinWebSearchToolName, nil, false, "serper", false},
		{BuiltinWebFetchToolName, nil, false, "jina", false},
		{BuiltinWebSearchToolName, nil, true, "deepseek", false},
		{BuiltinWebFetchToolName, nil, true, "deepseek", false},
		{BuiltinWebSearchToolName, map[string]string{"SERPER_API_KEY": "key"}, true, "serper", false},
		{BuiltinWebFetchToolName, map[string]string{"SERPER_API_KEY": "key"}, true, "jina", false},
		{BuiltinWebSearchToolName, map[string]string{"SERPER_API_KEY": "key", "WEB_SEARCH_BACKEND": "deepseek"}, true, "deepseek", false},
		{BuiltinWebFetchToolName, map[string]string{"WEB_FETCH_BACKEND": "jina"}, true, "jina", false},
		{BuiltinWebSearchToolName, map[string]string{"WEB_SEARCH_BACKEND": "deepseek"}, false, "", true},
		{BuiltinWebFetchToolName, map[string]string{"WEB_FETCH_BACKEND": "serper"}, true, "", true},
	} {
		got, err := WebBackend(tc.env, tc.tool, tc.available)
		require.Equal(t, tc.want, got)
		if tc.fail {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
}
