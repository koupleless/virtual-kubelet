package provider

import (
	"testing"
)

func TestVNode_ExitWhenLeaderChanged(t *testing.T) {
	//vnode := VNode{
	//	WhenLeaderAcquiredByOthers: make(chan struct{}),
	//}
	//select {
	//case <-vnode.ExitWhenLeaderChanged():
	//	assert.Fail(t, "ExitWhenLeaderChanged should have been called")
	//default:
	//}
	//vnode.leaderAcquiredByOthers()
	//select {
	//case <-vnode.ExitWhenLeaderChanged():
	//default:
	//	assert.Fail(t, "ExitWhenLeaderChanged should not called")
	//}
}
