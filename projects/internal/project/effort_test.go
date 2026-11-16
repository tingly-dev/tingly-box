package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidEffort(t *testing.T) {
	for _, level := range EffortLevels {
		assert.True(t, ValidEffort(level), "ladder level %q must be valid", level)
	}
	assert.False(t, ValidEffort("ultra"))
	assert.False(t, ValidEffort("minimal"), "collapsed server-side on most models, not offered")
}
