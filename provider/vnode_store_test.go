package provider

import (
	"github.com/koupleless/virtual-kubelet/model"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestNewVNodeStore(t *testing.T) {
	store := NewVNodeStore()
	assert.Assert(t, store != nil)
}

func TestPutBaseNode(t *testing.T) {
	store := NewVNodeStore()
	store.AddVNode("suite", &VNode{})
	assert.Assert(t, len(store.nodeNameToVNode) == 1)
}

func TestDeleteBaseNode(t *testing.T) {
	store := NewVNodeStore()
	store.AddVNode("suite", &VNode{})
	store.DeleteVNode("suite")
	assert.Assert(t, len(store.nodeNameToVNode) == 0)
}

func TestGetBaseNode(t *testing.T) {
	store := NewVNodeStore()
	store.AddVNode("suite", &VNode{})
	baseNode := store.GetVNode("suite")
	assert.Assert(t, baseNode != nil)
	store.DeleteVNode("suite")
	baseNode = store.GetVNode("suite")
	assert.Assert(t, baseNode == nil)
}

func TestGetBaseNodes(t *testing.T) {
	store := NewVNodeStore()
	store.AddVNode("suite", &VNode{})
	store.AddVNode("test2", &VNode{})
	nodes := store.GetVNodes()
	assert.Assert(t, len(nodes) == 2)
}

func TestPutBaseNodeNX(t *testing.T) {
	store := NewVNodeStore()
	err := store.AddVNode("suite", &VNode{})
	assert.Assert(t, err == nil)
	err = store.AddVNode("suite", &VNode{})
	assert.Assert(t, err != nil)
}

func TestGetBaseNodeByNodeName(t *testing.T) {
	store := NewVNodeStore()
	node := store.GetVNodeByNodeName("suite")
	assert.Assert(t, node == nil)
	store.AddVNode("suite", &VNode{})
	node = store.GetVNodeByNodeName("suite")
	assert.Assert(t, node != nil)
}

func TestGetLeaseOutdatedVNodeName(t *testing.T) {
	store := NewVNodeStore()
	clientId := "test-client-id"
	nodeName := "test-node-name"
	vNode := &VNode{
		name:     nodeName,
		env:      "test-env",
		Liveness: Liveness{},
	}

	vNode.lease = vNode.newLease(clientId)
	vNode.lease.Spec.RenewTime.Time = time.Now().Add(-time.Second * model.NodeLeaseDurationSeconds)
	store.AddVNode(nodeName, vNode)
	nameList := store.GetLeaseOutdatedVNodeNames(clientId)
	assert.Assert(t, len(nameList) == 1)
}
