package protocoltest

import "testing"

func TestEquivalentArguments(t *testing.T) {
	if !equivalentArguments(`{"city":"New York","n":1}`, `{ "n": 1, "city": "New York" }`) {
		t.Fatal("object key ordering should not matter")
	}
	if equivalentArguments(`{"city":"New York"}`, `{"city":"NewYork"}`) {
		t.Fatal("whitespace inside string was ignored")
	}
	if equivalentArguments(`[1,2]`, `[2,1]`) {
		t.Fatal("array order was ignored")
	}
}
