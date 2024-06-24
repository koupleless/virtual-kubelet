package helper

import (
	"github.com/pkg/errors"
	"gotest.tools/assert"
	"testing"
)

func TestMustReturnFirst(t *testing.T) {
	result := MustReturnFirst[int64](10, nil)
	assert.Equal(t, int64(10), result)
	defer func() {
		if r := recover(); r == nil {
			assert.Assert(t, true)
		}
	}()
	MustReturnFirst[int64](10, errors.New("test error"))
	assert.Assert(t, false)
}
