package telegram

import "testing"

func TestAddressedToOtherBot(t *testing.T) {
	for _, tt := range []struct {
		text, own string
		want      bool
	}{
		{"/new@otherbot", "tingly_bot", true},
		{"/dir@OtherBot 1", "Tingly_Bot", true},
		{"  /new@otherbot", "tingly_bot", true},
		{"/new@tingly_bot", "tingly_bot", false},
		{"/new@Tingly_Bot hi", "tingly_bot", false},
		{"/new", "tingly_bot", false},
		{"mail me@otherbot", "tingly_bot", false},
		{"/new@otherbot", "", false},
	} {
		if got := addressedToOtherBot(tt.text, tt.own); got != tt.want {
			t.Errorf("addressedToOtherBot(%q, %q) = %v, want %v", tt.text, tt.own, got, tt.want)
		}
	}
}
