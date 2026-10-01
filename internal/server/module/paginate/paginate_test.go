package paginate

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func ctx(query string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/x?"+query, nil)
	return c
}

func TestLimit(t *testing.T) {
	cases := []struct {
		query    string
		def, max int
		want     int
	}{
		{"", 100, 500, 100},
		{"limit=20", 100, 500, 20},
		{"limit=0", 100, 500, 100},
		{"limit=-3", 100, 500, 100},
		{"limit=abc", 100, 500, 100},
		{"limit=9999", 100, 500, 500},
		{"limit=9999", 100, 0, 9999},
	}
	for _, tc := range cases {
		if got := Limit(ctx(tc.query), tc.def, tc.max); got != tc.want {
			t.Errorf("Limit(%q, %d, %d) = %d, want %d", tc.query, tc.def, tc.max, got, tc.want)
		}
	}
}

func TestOffset(t *testing.T) {
	for q, want := range map[string]int{"": 0, "offset=7": 7, "offset=-1": 0, "offset=x": 0, "offset=0": 0} {
		if got := Offset(ctx(q)); got != want {
			t.Errorf("Offset(%q) = %d, want %d", q, got, want)
		}
	}
}
