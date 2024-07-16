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
	assert.Assert(t, len(store.baseIDToKouplelessNode) == 1)
}

func TestRuntimeInfoStore_DeleteKouplelessNode(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutKouplelessNode("test", &node.KouplelessNode{})
	store.DeleteKouplelessNode("test")
	assert.Assert(t, len(store.baseIDToKouplelessNode) == 0)
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

func TestRuntimeInfoStore_BaseMsgArrived(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.BaseMsgArrived("test")
	assert.Assert(t, len(store.baseLatestMsgTime) == 1)
}

func TestRuntimeInfoStore_GetKouplelessNodes(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutKouplelessNode("test", &node.KouplelessNode{})
	store.PutKouplelessNode("test2", &node.KouplelessNode{})
	nodes := store.GetKouplelessNodes()
	assert.Assert(t, len(nodes) == 2)
}

func TestRuntimeInfoStore_GetOfflineBases(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.BaseMsgArrived("test")
	time.Sleep(time.Millisecond * 100)
	Bases := store.GetOfflineBases(50)
	assert.Assert(t, len(Bases) == 1)
}

func TestRuntimeInfoStore_PutKouplelessNodeNX(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutKouplelessNode("test", &node.KouplelessNode{})
	err := store.PutKouplelessNodeNX("test", &node.KouplelessNode{})
	assert.Assert(t, err != nil)
}
