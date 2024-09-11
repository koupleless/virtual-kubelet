package vnode_controller

import (
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/vnode"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestNewRuntimeInfoStore(t *testing.T) {
	store := NewRuntimeInfoStore()
	assert.Assert(t, store != nil)
}

func TestRuntimeInfoStore_PutBaseNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutVNode("suite", &vnode.VNode{})
	assert.Assert(t, len(store.nodeIDToVNode) == 1)
}

func TestRuntimeInfoStore_DeleteBaseNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutVNode("suite", &vnode.VNode{})
	store.DeleteVNode("suite")
	assert.Assert(t, len(store.nodeIDToVNode) == 0)
}

func TestRuntimeInfoStore_GetBaseNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutVNode("suite", &vnode.VNode{})
	baseNode := store.GetVNode("suite")
	assert.Assert(t, baseNode != nil)
	store.DeleteVNode("suite")
	baseNode = store.GetVNode("suite")
	assert.Assert(t, baseNode == nil)
}

func TestRuntimeInfoStore_BaseMsgArrived(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.NodeMsgArrived("suite")
	assert.Assert(t, len(store.vnodeLatestMsgTime) == 1)
}

func TestRuntimeInfoStore_GetBaseNodes(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutVNode("suite", &vnode.VNode{})
	store.PutVNode("test2", &vnode.VNode{})
	nodes := store.GetVNodes()
	assert.Assert(t, len(nodes) == 2)
}

func TestRuntimeInfoStore_GetOfflineBases(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.NodeMsgArrived("suite")
	time.Sleep(time.Millisecond * 100)
	store.NodeMsgArrived("suite-1")
	Bases := store.GetOfflineNodes(50)
	assert.Assert(t, len(Bases) == 1)
}

func TestRuntimeInfoStore_PutBaseNodeNX(t *testing.T) {
	store := NewRuntimeInfoStore()
	err := store.PutVNodeIDNX("suite")
	assert.Assert(t, err == nil)
	err = store.PutVNodeIDNX("suite")
	assert.Assert(t, err != nil)
}

func TestRuntimeInfoStore_GetBaseNodeByNodeID(t *testing.T) {
	store := NewRuntimeInfoStore()
	node := store.GetVNodeByNodeName("suite")
	assert.Assert(t, node == nil)
	store.PutVNode("suite", &vnode.VNode{})
	node = store.GetVNodeByNodeName(utils.FormatNodeName("suite"))
	assert.Assert(t, node != nil)
}
