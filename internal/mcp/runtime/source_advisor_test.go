package runtime

import (
	"strings"
	"testing"
)

func TestNormalizeAdvisorResponse(t *testing.T) {
	valid := `{"assessment":"ok","recommendation":"do it"}`
	got := normalizeAdvisorResponse(valid)
	if got != valid {
		t.Errorf("normalizeAdvisorResponse(valid)=%q, want %q", got, valid)
	}

	invalid := "just some text"
	got = normalizeAdvisorResponse(invalid)
	if !strings.Contains(got, "non-JSON") || !strings.Contains(got, "just some text") {
		t.Errorf("normalizeAdvisorResponse(invalid) unexpected: %q", got)
	}
}
