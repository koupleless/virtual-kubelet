package virtual_kubelet

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestMinDuration(t *testing.T) {
	assert.Equal(t, time.Second, minDuration(time.Second, time.Second*2))
	assert.Equal(t, time.Second, minDuration(time.Second*2, time.Second))
}
