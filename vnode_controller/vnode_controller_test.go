package vnode_controller

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/provider"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{}, &tunnel.MockTunnel{})
	assert.NotNil(t, err)
}

func TestNewVNodeController_Success(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodType:  "suite",
		KubeCache: &informertest.FakeInformers{},
		IsCluster: true,
	}, &tunnel.MockTunnel{})
	assert.Nil(t, err)
}

func TestDiscoverPreviousNode(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}

	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		KubeCache: &informertest.FakeInformers{},
		VPodType:  "suite",
	}, &mockTunnel)

	nodeList := &corev1.NodeList{
		Items: []corev1.Node{
			{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-node-without-tunnel",
					Labels: map[string]string{
						model.LabelKeyOfComponent:   model.ComponentVNode,
						model.LabelKeyOfEnv:         "suite",
						model.LabelKeyOfBaseName:    "test-cluster-name",
						model.LabelKeyOfBaseVersion: "1.0.0",
					},
				},
			},
			{
				ObjectMeta: v1.ObjectMeta{
					Name:   "vnode.test-node-with-tunnel",
					Labels: map[string]string{},
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
	}

	vc.client = fake.NewFakeClient(&nodeList.Items[0], &nodeList.Items[1])
	vc.cache = &informertest.FakeInformers{}
	vc.discoverPreviousNodes(nodeList)
	time.Sleep(20 * time.Second)
	assert.Equal(t, 2, len(vc.vNodeStore.GetVNodes()))
}

func TestRunVNode(t *testing.T) {
	// init mocked vNodeController
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		KubeCache: &informertest.FakeInformers{},
		VPodType:  "suite",
		ClientID:  "mockClientID",
	}, &mockTunnel)

	node := corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name: "vnode.test-node-with-tunnel",
			Labels: map[string]string{
				model.LabelKeyOfBaseVersion:     "1.0.0",
				model.LabelKeyOfBaseClusterName: "vnode-test-cluster-name",
				model.LabelKeyOfBaseName:        "mockVNode",
				model.LabelKeyOfEnv:             "dev",
				model.LabelKeyOfComponent:       "vnode",
			},
			Annotations: map[string]string{},
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
	}

	nodeInfo := utils.ConvertNodeToNodeInfo(&node)

	vc.client = fake.NewFakeClient(&node)
	vc.cache = &informertest.FakeInformers{}

	// init mocked vNode
	vnCtx, vnCtxCancel := context.WithCancel(context.WithValue(context.Background(), "nodeName", node.Name))
	vNode, _ := vc.createVNode(vnCtx, nodeInfo)
	vNode.Liveness.UpdateHeartBeatTime()

	// runVNode
	vc.runVNode(vnCtx, vNode, nodeInfo)

	// case1: leader election success, take over the vnode and run
	time.Sleep(5 * time.Second)
	assert.True(t, vc.takeOveredVNodeName.Contains("vnode.test-node-with-tunnel"))
	assert.Equal(t, 1, len(vc.vNodeStore.GetVNodes()))

	// case2: leader election success, take over the vnode but vnode is dead, which triggers vnCtxCancel()
	vnCtxCancel()
	time.Sleep(5 * time.Second)
	assert.False(t, vc.takeOveredVNodeName.Contains("vnode.test-node-with-tunnel"))
}

func TestDiscoverPreviousPods(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		KubeCache: &informertest.FakeInformers{},
		VPodType:  "suite",
	}, &mockTunnel)
	vn := &provider.VNode{
		//tunnel: &mockTunnel,
	}
	vc.vNodeStore.AddVNode("test-node", vn)
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
		VPodType:  "suite",
		KubeCache: &informertest.FakeInformers{},
	}, &mockTunnel)

	result, err := vc.Reconcile(nil, reconcile.Request{})
	assert.Nil(t, err)
	assert.Equal(t, reconcile.Result{}, result)
}

func TestCallBack_NoVnode(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodType:  "suite",
		KubeCache: &informertest.FakeInformers{},
	}, &mockTunnel)

	vc.onBaseStatusArrived("test", model.NodeStatusData{})
	vc.onAllBizStatusArrived("test", nil)
	vc.onSingleBizStatusArrived("test", model.BizStatusData{})
	vc.onBaseStatusArrived("test", model.NodeStatusData{})
}

func TestPodHandler_NoVnodeOrNotLeader(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodType:  "suite",
		KubeCache: &informertest.FakeInformers{},
	}, &mockTunnel)

	ctx := context.TODO()

	vc.podAddHandler(ctx, &corev1.Pod{
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

	vc.vNodeStore.AddVNode("test-node", &provider.VNode{})
	vc.podAddHandler(ctx, &corev1.Pod{
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
		VPodType:  "suite",
		KubeCache: &informertest.FakeInformers{},
	}, &mockTunnel)

	level := vc.workloadLevel()
	assert.Equal(t, 0, level)
	vc.vNodeStore.AddVNode("test-node", &provider.VNode{})
	level = vc.workloadLevel()
	assert.Equal(t, 0, level)
}

func TestDelayWithWorkload(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		KubeCache: &informertest.FakeInformers{},
		VPodType:  "suite",
	}, &mockTunnel)
	now := time.Now()
	vc.delayWithWorkload(context.TODO())
	vc.isCluster = true
	vc.delayWithWorkload(context.TODO())
	end := time.Now()
	ctx, cancelFunc := context.WithTimeout(context.TODO(), time.Millisecond*20)
	cancelFunc()
	vc.vNodeStore.UpdateNodeStateOnProviderArrived("test-node", model.NodeStateActivated)
	vc.vNodeStore.AddVNode("test-node", &provider.VNode{})
	vc.delayWithWorkload(ctx)
	assert.True(t, end.Sub(now) < time.Millisecond*100)
}

func TestShutdownNonExistVNode(t *testing.T) {
	mockTunnel := tunnel.MockTunnel{}
	vc, _ := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodType:  "suite",
		KubeCache: &informertest.FakeInformers{},
	}, &mockTunnel)
	vc.shutdownVNode("test-node")
}

func TestDeleteGraceTimeEqual(t *testing.T) {
	assert.True(t, deleteGraceTimeEqual(nil, nil))
	assert.False(t, deleteGraceTimeEqual(ptr.To[int64](1), nil))
	assert.True(t, deleteGraceTimeEqual(ptr.To[int64](1), ptr.To[int64](1)))
	assert.False(t, deleteGraceTimeEqual(ptr.To[int64](1), ptr.To[int64](2)))
}

func TestPodShouldEnqueue(t *testing.T) {
	assert.False(t, podShouldEnqueue(nil, nil))
	assert.False(t, podShouldEnqueue(&corev1.Pod{}, nil))
	assert.True(t, podShouldEnqueue(&corev1.Pod{}, &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{
				"suite": "suite",
			},
		},
	}))
	assert.True(t, podShouldEnqueue(&corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			DeletionGracePeriodSeconds: ptr.To[int64](1),
		},
	}, &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			DeletionGracePeriodSeconds: ptr.To[int64](2),
		},
	}))
	assert.True(t, podShouldEnqueue(&corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			DeletionTimestamp: &v1.Time{
				Time: time.UnixMilli(1),
			},
		},
	}, &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			DeletionTimestamp: &v1.Time{
				Time: time.UnixMilli(2),
			},
		},
	}))
	assert.False(t, podShouldEnqueue(&corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			DeletionTimestamp: &v1.Time{
				Time: time.UnixMilli(1),
			},
		},
	}, &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			DeletionTimestamp: &v1.Time{
				Time: time.UnixMilli(1),
			},
		},
	}))
}
