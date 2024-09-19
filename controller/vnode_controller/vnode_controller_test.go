package vnode_controller

import (
	"context"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/vnode"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestNewVNodeController_NoConfig(t *testing.T) {
	_, err := NewVNodeController(nil, nil)
	assert.NotNil(t, err)
}

func TestNewVNodeController_ConfigNoTunnels(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{}, nil)
	assert.NotNil(t, err)
}

func TestNewVNodeController_ConfigNoIdentity(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{}, []tunnel.Tunnel{
		&tunnel.MockTunnel{},
	})
	assert.NotNil(t, err)
}

func TestNewVNodeController_Success(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&tunnel.MockTunnel{},
	})
	assert.Nil(t, err)
}

func TestDiscoverPreviousNode(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})
	vc.discoverPreviousNodes(context.TODO(), &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: v1.ObjectMeta{
					Name:   "test-node-without-tunnel",
					Labels: map[string]string{},
				},
			},
			{
				ObjectMeta: v1.ObjectMeta{
					Name: "vnode.test-node-with-tunnel",
					Labels: map[string]string{
						model.LabelKeyOfVnodeTunnel: mockTunnel.Key(),
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "10.0.0.1",
						},
						{
							Type:    corev1.NodeHostName,
							Address: "test-node",
						},
					},
				},
			},
		},
	})
	assert.Equal(t, len(vc.runtimeInfoStore.nodeIDLock), 1)
}

func TestDiscoverPreviousPods(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})
	vc.runtimeInfoStore.PutVNode("test-node", &vnode.VNode{
		Tunnel: &mockTunnel,
	})
	vc.discoverPreviousPods(context.TODO(), &corev1.PodList{
		Items: []corev1.Pod{
			{
				Spec: corev1.PodSpec{
					NodeName: "vnode.test-node-not-exist",
				},
			},
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
				},
				Spec: corev1.PodSpec{
					NodeName: "vnode.test-node",
					Containers: []corev1.Container{
						{
							Name: "test-container",
						},
					},
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "test-container",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{
									StartedAt: v1.Now(),
								},
							},
						},
					},
				},
			},
		},
	})
}
