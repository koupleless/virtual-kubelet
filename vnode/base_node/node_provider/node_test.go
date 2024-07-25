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
		BizName:   "test",
		Version:   "1.0.0",
	})
	assert.Assert(t, vnode != nil)
}

func TestVirtualKubeletNode_Register(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		BizName:   "test",
		Version:   "1.0.0",
	})
	node := &corev1.Node{}
	err := vnode.Register(context.Background(), node)
	assert.NilError(t, err)
	assert.Assert(t, len(node.Labels) == 3)
	assert.Assert(t, len(node.Spec.Taints) == 1)
	assert.Assert(t, node.Status.Phase == corev1.NodePending)
}

func TestVirtualKubeletNode_Ping(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		BizName:   "test",
		Version:   "1.0.0",
	})
	err := vnode.Ping(context.Background())
	assert.NilError(t, err)
}

func TestVirtualKubeletNode_NotifyNodeStatus(t *testing.T) {
	vnode := NewVirtualKubeletNode(BuildBaseNodeProviderConfig{
		NodeIP:    "127.0.0.1",
		TechStack: "java",
		BizName:   "test",
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
	vnode.Notify(ark.HealthData{})
	assert.Assert(t, len(nodeList) == 1)
}
