package node_provider

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"testing"
)

func TestNewVirtualKubeletNode(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		Name:      "test",
		Version:   "1.0.0",
	})
	assert.Assert(t, vnode != nil)
}

func TestVirtualKubeletNode_Register(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		Name:      "test",
		Version:   "1.0.0",
		Env:       "test",
	})
	node := &corev1.Node{}
	err := vnode.Register(node, "test")
	assert.NilError(t, err)
	assert.Assert(t, len(node.Spec.Taints) == 2)
	assert.Assert(t, node.Status.Phase == corev1.NodePending)
}

func TestVirtualKubeletNode_Ping(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		Name:      "test",
		Version:   "1.0.0",
	})
	err := vnode.Ping(context.Background())
	assert.NilError(t, err)
}

func TestVirtualKubeletNode_NotifyNodeStatus(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		Name:      "test",
		Version:   "1.0.0",
	})
	vnode.nodeInfo = &corev1.Node{
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{},
			Allocatable: corev1.ResourceList{},
		},
	}
	nodeList := make([]*corev1.Node, 0)
	vnode.NotifyNodeStatus(context.Background(), func(node *corev1.Node) {
		nodeList = append(nodeList, node)
	})
	vnode.Notify(ark.HealthData{
		Jvm: ark.JvmInfo{
			JavaUsedMetaspace:      10000,
			JavaCommittedMetaspace: 10000,
			JavaMaxMetaspace:       100000,
		},
	})
	assert.Assert(t, len(nodeList) == 1)
}

func TestVirtualKubeletNode_NotifyNodeStatusWithoutInit(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		Name:      "test",
		Version:   "1.0.0",
	})
	nodeList := make([]*corev1.Node, 0)
	vnode.NotifyNodeStatus(context.Background(), func(node *corev1.Node) {
		nodeList = append(nodeList, node)
	})
	vnode.Notify(ark.HealthData{})
	assert.Assert(t, len(nodeList) == 0)
}

func TestBaseNodeProvider_CurrNodeInfo(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		Name:      "test",
		Version:   "1.0.0",
		Env:       "test",
	})
	vnode.Register(&corev1.Node{}, "test")
	assert.Assert(t, vnode.CurrNodeInfo() != nil)
}
