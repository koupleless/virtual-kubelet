package base_register_controller

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
	"testing"
)

func TestDeleteGraceTimeEqual(t *testing.T) {
	assert.True(t, deleteGraceTimeEqual(nil, nil))
	assert.False(t, deleteGraceTimeEqual(ptr.To[int64](1), nil))
	assert.True(t, deleteGraceTimeEqual(ptr.To[int64](1), ptr.To[int64](1)))
	assert.False(t, deleteGraceTimeEqual(ptr.To[int64](1), ptr.To[int64](2)))
}
