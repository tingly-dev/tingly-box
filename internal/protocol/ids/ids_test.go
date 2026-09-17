package ids

import (
	"regexp"
	"testing"
)

func TestShape(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]+_[0-9a-f]{32}$`)
	for _, id := range []string{Response(), Message(), FunctionCall(), Reasoning(), Call()} {
		if !re.MatchString(id) {
			t.Fatalf("id %q is not <prefix>_<32 hex>", id)
		}
		if len(id) > 64 {
			t.Fatalf("id %q exceeds 64 chars", id)
		}
	}
	if Message() == Message() {
		t.Fatal("ids must not repeat")
	}
}

func TestSetGeneratorForTest(t *testing.T) {
	n := 0
	restore := SetGeneratorForTest(func() string { n++; return "fixed" + string(rune('0'+n)) })
	if got := FunctionCall(); got != "fc_fixed1" {
		t.Fatalf("got %q", got)
	}
	restore()
	if got := FunctionCall(); len(got) != len("fc_")+32 {
		t.Fatalf("generator not restored: %q", got)
	}
}
