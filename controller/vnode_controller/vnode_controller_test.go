package vnode_controller

import (
	"context"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/vnode"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
	"time"
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
		IsCluster:    true,
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
	vc.discoverPreviousNodes(&corev1.NodeList{
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
	assert.Equal(t, len(vc.runtimeInfoStore.startLock), 1)
}

func TestDiscoverPreviousPods(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})
	vn := &vnode.VNode{
		Tunnel: &mockTunnel,
	}
	vc.runtimeInfoStore.PutVNode("test-node", vn)
	vc.discoverPreviousPods(context.TODO(), vn, &corev1.PodList{
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

func TestReconcile(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})

	result, err := vc.Reconcile(nil, reconcile.Request{})
	assert.Nil(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

func TestCallBack_NoVnode(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})

	vc.onNodeStatusDataArrived("test", model.NodeStatusData{})
	vc.onQueryAllContainerStatusDataArrived("test", nil)
	vc.onContainerStatusChanged("test", model.BizStatusData{})
	vc.onNodeStatusDataArrived("test", model.NodeStatusData{})
}

func TestPodHandler_NoVnodeOrNotLeader(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})

	ctx := context.TODO()

	vc.podCreateHandler(ctx, &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "vnode.test-node.env",
		},
	})
	vc.podUpdateHandler(ctx, &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "vnode.test-node.env",
		},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "vnode.test-node.env",
		},
	})
	vc.podDeleteHandler(ctx, &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "vnode.test-node.env",
		},
	})

	vc.runtimeInfoStore.PutVNode("test-node", &vnode.VNode{})
	vc.podCreateHandler(ctx, &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "vnode.test-node.env",
		},
	})
	vc.podUpdateHandler(ctx, &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "vnode.test-node.env",
		},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "vnode.test-node.env",
		},
	})
	vc.podDeleteHandler(ctx, &corev1.Pod{
		Spec: corev1.PodSpec{
			NodeName: "vnode.test-node.env",
		},
	})
}

func TestWorkloadLevel(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})

	level := vc.workloadLevel()
	assert.Equal(t, 0, level)
	vc.runtimeInfoStore.PutNode("test-node")
	level = vc.workloadLevel()
	assert.Equal(t, 0, level)
}

func TestDelayWithWorkload(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})
	now := time.Now()
	vc.delayWithWorkload(context.TODO())
	vc.isCluster = true
	vc.delayWithWorkload(context.TODO())
	end := time.Now()
	ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Millisecond*20)
	cancelFunc()
	vc.runtimeInfoStore.NodeRunning("test-node")
	vc.runtimeInfoStore.PutNode("test-node")
	vc.delayWithWorkload(ctx)
	assert.True(t, end.Sub(now) < time.Millisecond*100)
}

func TestShutdownNonExistVNode(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})
	vc.shutdownVNode("test-node")
}

func TestWakeUpNonExistVNode(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&mockTunnel,
	})
	vc.wakeUpVNode(context.TODO(), "test-node")
}
