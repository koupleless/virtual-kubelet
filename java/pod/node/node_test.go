package node

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"os"
	"testing"
	"time"
)

var vnode *VirtualKubeletNode

func TestNewVirtualKubeletNode(t *testing.T) {
	os.Setenv("TECH_STACK", "java")
	os.Setenv("VNODE_POD_CAPACITY", "2")
	os.Setenv("BASE_POD_IP", "127.0.0.1")
	os.Setenv("VNODE_VERSION", "0.0.1")
	arkService := ark.BuildService(context.Background())
	vnode = NewVirtualKubeletNode(arkService)
	assert.Assert(t, vnode != nil)
	assert.Assert(t, vnode.nodeConfig.NodeIP == "127.0.0.1")
	assert.Assert(t, vnode.nodeConfig.VPodCapacity == 2)
	assert.Assert(t, vnode.nodeConfig.Version == "0.0.1")
	assert.Assert(t, vnode.nodeConfig.TechStack == "java")
}

func TestVirtualKubeletNode_Register(t *testing.T) {
	node := &corev1.Node{}
	err := vnode.Register(context.Background(), node)
	assert.NilError(t, err)
	assert.Assert(t, len(node.Labels) == 2)
	assert.Assert(t, len(node.Spec.Taints) == 1)
	assert.Assert(t, node.Status.Phase == corev1.NodeRunning)
	quantity, has := node.Status.Capacity[corev1.ResourcePods]
	assert.Assert(t, has)
	assert.Assert(t, quantity.Value() == 2)
}

func TestVirtualKubeletNode_Ping(t *testing.T) {
	err := vnode.Ping(context.Background())
	assert.NilError(t, err)
}

func TestVirtualKubeletNode_NotifyNodeStatus(t *testing.T) {
	nodeList := make([]*corev1.Node, 0)
	vnode.NotifyNodeStatus(context.Background(), func(node *corev1.Node) {
		nodeList = append(nodeList, node)
	})
	time.Sleep(3200 * time.Millisecond)
	assert.Assert(t, len(nodeList) == 1)
}
