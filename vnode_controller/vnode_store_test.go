package vnode_controller

import (
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/provider"
	"gotest.tools/assert"
	"testing"
)

func TestNewVNodeStore(t *testing.T) {
	store := NewVNodeStore()
	assert.Assert(t, store != nil)
}

func TestVNodeStore_PutBaseNode(t *testing.T) {
	store := NewVNodeStore()
	store.AddVNode("suite", &provider.VNode{})
	assert.Assert(t, len(store.nodeIDToVNode) == 1)
}

func TestVNodeStore_DeleteBaseNode(t *testing.T) {
	store := NewVNodeStore()
	store.AddVNode("suite", &provider.VNode{})
	store.DeleteVNode("suite")
	assert.Assert(t, len(store.nodeIDToVNode) == 0)
}

func TestVNodeStore_GetBaseNode(t *testing.T) {
	store := NewVNodeStore()
	store.AddVNode("suite", &provider.VNode{})
	baseNode := store.GetVNode("suite")
	assert.Assert(t, baseNode != nil)
	store.DeleteVNode("suite")
	baseNode = store.GetVNode("suite")
	assert.Assert(t, baseNode == nil)
}

func TestVNodeStore_GetBaseNodes(t *testing.T) {
	store := NewVNodeStore()
	store.AddVNode("suite", &provider.VNode{})
	store.AddVNode("test2", &provider.VNode{})
	nodes := store.GetVNodes()
	assert.Assert(t, len(nodes) == 2)
}

func TestVNodeStore_PutBaseNodeNX(t *testing.T) {
	store := NewVNodeStore()
	err := store.AddVNode("suite", &provider.VNode{})
	assert.Assert(t, err == nil)
	err = store.AddVNode("suite", &provider.VNode{})
	assert.Assert(t, err != nil)
}

func TestVNodeStore_GetBaseNodeByNodeID(t *testing.T) {
	store := NewVNodeStore()
	node := store.GetVNodeByNodeName("suite")
	assert.Assert(t, node == nil)
	store.AddVNode("suite", &provider.VNode{})
	node = store.GetVNodeByNodeName(utils.FormatNodeName("suite", "suite"))
	assert.Assert(t, node != nil)
}
