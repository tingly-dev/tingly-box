package obs

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestLevelForStatus(t *testing.T) {
	cases := []struct {
		status int
		want   logrus.Level
	}{
		{200, logrus.InfoLevel},
		{201, logrus.InfoLevel},
		{399, logrus.InfoLevel},
		{400, logrus.WarnLevel},
		{404, logrus.WarnLevel},
		{429, logrus.WarnLevel},
		{499, logrus.WarnLevel},
		{500, logrus.ErrorLevel},
		{503, logrus.ErrorLevel},
	}
	for _, c := range cases {
		if got := LevelForStatus(c.status); got != c.want {
			t.Errorf("LevelForStatus(%d) = %v, want %v", c.status, got, c.want)
		}
	}
}
