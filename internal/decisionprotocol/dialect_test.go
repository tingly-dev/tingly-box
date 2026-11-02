package decisionprotocol

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRequest(t *testing.T) {
	for _, tc := range []struct {
		body    string
		dialect Dialect
		invalid bool
	}{
		{`{"input":"hi","questions":[{"type":"predicate"}]}`, OpenAI, false},
		{`{"state":"hi","questions":{"q":{"type":"noul"}}}`, SystemOne, false},
		{`{"model":"legacy","options":["a"]}`, Unknown, false},
		{`{"input":"x","state":"y"}`, Unknown, true},
		{`{"input":"x","questions":{}}`, Unknown, true},
		{`{"state":"x","questions":[]}`, Unknown, true},
		{`{"questions":null}`, Unknown, true},
		{`[]`, Unknown, true},
	} {
		got, err := Request([]byte(tc.body))
		if tc.invalid {
			require.Error(t, err, tc.body)
		} else {
			require.NoError(t, err, tc.body)
			require.Equal(t, tc.dialect, got, tc.body)
		}
	}
}
func TestResponse(t *testing.T) {
	require.Equal(t, OpenAI, Response([]byte(`{"answers":[]}`)))
	require.Equal(t, SystemOne, Response([]byte(`{"answers":{}}`)))
	require.Equal(t, Unknown, Response([]byte(`{"answer":"a"}`)))
}
