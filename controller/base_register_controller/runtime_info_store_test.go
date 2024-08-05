package base_register_controller

import (
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/vnode/base_node"
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
	store.PutBaseNode("test", &base_node.BaseNode{})
	assert.Assert(t, len(store.baseIDToBaseNode) == 1)
}

func TestRuntimeInfoStore_DeleteBaseNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutBaseNode("test", &base_node.BaseNode{})
	store.DeleteBaseNode("test")
	assert.Assert(t, len(store.baseIDToBaseNode) == 0)
}

func TestRuntimeInfoStore_GetBaseNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutBaseNode("test", &base_node.BaseNode{})
	baseNode := store.GetBaseNode("test")
	assert.Assert(t, baseNode != nil)
	store.DeleteBaseNode("test")
	baseNode = store.GetBaseNode("test")
	assert.Assert(t, baseNode == nil)
}

func TestRuntimeInfoStore_BaseMsgArrived(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.BaseMsgArrived("test")
	assert.Assert(t, len(store.baseLatestMsgTime) == 1)
}

func TestRuntimeInfoStore_GetBaseNodes(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutBaseNode("test", &base_node.BaseNode{})
	store.PutBaseNode("test2", &base_node.BaseNode{})
	nodes := store.GetBaseNodes()
	assert.Assert(t, len(nodes) == 2)
}

func TestRuntimeInfoStore_GetOfflineBases(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.BaseMsgArrived("test")
	time.Sleep(time.Millisecond * 100)
	store.BaseMsgArrived("test-1")
	Bases := store.GetOfflineBases(50)
	assert.Assert(t, len(Bases) == 1)
}

func TestRuntimeInfoStore_PutBaseNodeNX(t *testing.T) {
	store := NewRuntimeInfoStore()
	err := store.PutBaseIDNX("test")
	assert.Assert(t, err == nil)
	err = store.PutBaseIDNX("test")
	assert.Assert(t, err != nil)
}

func TestRuntimeInfoStore_GetBaseNodeByNodeID(t *testing.T) {
	store := NewRuntimeInfoStore()
	node := store.GetBaseNodeByNodeName("test")
	assert.Assert(t, node == nil)
	store.PutBaseNode("test", &base_node.BaseNode{})
	node = store.GetBaseNodeByNodeName(utils.FormatBaseNodeName("test"))
	assert.Assert(t, node != nil)
}
