package provider

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestVNode_ExitWhenLeaderChanged(t *testing.T) {
	vnode := VNode{
		exitWhenLeaderChanged: make(chan struct{}),
	}
	select {
	case <-vnode.ExitWhenLeaderChanged():
		assert.Fail(t, "ExitWhenLeaderChanged should have been called")
	default:
	}
	vnode.leaderChanged()
	select {
	case <-vnode.ExitWhenLeaderChanged():
	default:
		assert.Fail(t, "ExitWhenLeaderChanged should not called")
	}
}
