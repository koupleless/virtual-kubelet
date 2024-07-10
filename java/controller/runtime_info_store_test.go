package controller

import (
	"github.com/koupleless/virtual-kubelet/java/pod/node"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestNewRuntimeInfoStore(t *testing.T) {
	store := NewRuntimeInfoStore()
	assert.Assert(t, store != nil)
}

func TestRuntimeInfoStore_PutKouplelessNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutKouplelessNode("test", &node.KouplelessNode{})
	assert.Assert(t, len(store.deviceIDToKouplelessNode) == 1)
}

func TestRuntimeInfoStore_DeleteKouplelessNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutKouplelessNode("test", &node.KouplelessNode{})
	store.DeleteKouplelessNode("test")
	assert.Assert(t, len(store.deviceIDToKouplelessNode) == 0)
}

func TestRuntimeInfoStore_GetKouplelessNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutKouplelessNode("test", &node.KouplelessNode{})
	kouplelessNode := store.GetKouplelessNode("test")
	assert.Assert(t, kouplelessNode != nil)
	store.DeleteKouplelessNode("test")
	kouplelessNode = store.GetKouplelessNode("test")
	assert.Assert(t, kouplelessNode == nil)
}

func TestRuntimeInfoStore_DeviceMsgArrived(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.DeviceMsgArrived("test")
	assert.Assert(t, len(store.deviceLatestMsgTime) == 1)
}

func TestRuntimeInfoStore_GetKouplelessNodes(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutKouplelessNode("test", &node.KouplelessNode{})
	store.PutKouplelessNode("test2", &node.KouplelessNode{})
	nodes := store.GetKouplelessNodes()
	assert.Assert(t, len(nodes) == 2)
}

func TestRuntimeInfoStore_GetOfflineDevices(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.DeviceMsgArrived("test")
	time.Sleep(time.Millisecond * 100)
	devices := store.GetOfflineDevices(50)
	assert.Assert(t, len(devices) == 1)
}

func TestRuntimeInfoStore_PutKouplelessNodeNX(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutKouplelessNode("test", &node.KouplelessNode{})
	err := store.PutKouplelessNodeNX("test", &node.KouplelessNode{})
	assert.Assert(t, err != nil)
}
