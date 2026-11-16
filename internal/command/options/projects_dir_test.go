package options

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultProjectsDir(t *testing.T) {
	assert.Equal(t, filepath.Join("home", ".tingly-box-project"), DefaultProjectsDir(filepath.Join("home", ".tingly-box")))
	assert.Equal(t, filepath.Join("work", "tb-dev-project"), DefaultProjectsDir(filepath.Join("work", "tb-dev")+"/"))
}
