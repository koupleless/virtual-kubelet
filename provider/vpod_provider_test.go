package provider

import (
	"context"
	"testing"
	"time"

	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	crtfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncRelatedPodStatus(t *testing.T) {
	provider := NewVPodProvider("default", "127.0.0.1", "123", nil, nil, &tunnel.MockTunnel{})
	provider.syncBizStatusToKube(context.TODO(), model.BizStatusData{
		Key:        "test-biz-key",
		Name:       "test-name",
		PodKey:     "test-pod-key",
		State:      "test-state",
		ChangeTime: time.Now(),
		Reason:     "test-reason",
		Message:    "test-message",
	})
}

func TestSyncAllContainerInfo(t *testing.T) {
	tl := &tunnel.MockTunnel{}
	provider := NewVPodProvider("default", "127.0.0.1", "123", nil, &informertest.FakeInformers{}, tl)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "test-image",
				},
			},
		},
	}
	provider.vPodStore.podKeyToPod = map[string]*corev1.Pod{
		"test": pod,
		"test2": {
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: time.Now().Add(time.Second)},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test-container-2",
						Image: "test-image",
					},
				},
			},
		},
	}
	provider.SyncAllBizStatusToKube(context.TODO(), []model.BizStatusData{
		{
			Key: tl.GetBizUniqueKey(&corev1.Container{
				Name: "test-container",
			}),
			PodKey: "namespace/name",
		},
	})
}

func TestUpdateDeletedPod(t *testing.T) {
	tl := &tunnel.MockTunnel{}
	provider := NewVPodProvider("default", "127.0.0.1", "123", nil, nil, tl)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			CreationTimestamp: metav1.Time{Time: time.Now()},
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "test-image",
				},
			},
		},
	}
	err := provider.UpdatePod(context.TODO(), pod)
	assert.NoError(t, err)
}

func TestResyncPod(t *testing.T) {
	tl := tunnel.NewMockTunnel()
	onBizStartCh := make(chan model.BizStatusData, 1)
	tl.OnSingleBizStatusArrived = func(nodeName string, data model.BizStatusData) {
		onBizStartCh <- data
	}
	podInStore := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			Namespace:         "test-namespace",
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "test-image",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  "test-container",
					Image: "test-image",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  model.StateReasonAwaitingResync,
							Message: "Test resync container",
						},
					},
				},
			},
		},
	}
	podInK8s := podInStore.DeepCopy()
	podInK8s.Status.Phase = corev1.PodRunning
	podInK8s.Status.ContainerStatuses[0].State = corev1.ContainerState{
		Running: &corev1.ContainerStateRunning{},
	}

	fakeCli := crtfake.NewFakeClient(podInK8s)

	provider := NewVPodProvider("default", "127.0.0.1", "test-node", fakeCli, nil, tl)
	provider.vPodStore.podKeyToPod = map[string]*corev1.Pod{
		"test-namespace/test-pod": podInStore,
	}
	onK8sUpdateCh := make(chan *corev1.Pod, 1)
	provider.NotifyPods(context.Background(), func(pod *corev1.Pod) {
		onK8sUpdateCh <- pod
	})

	err := provider.UpdatePod(context.TODO(), podInStore)
	assert.NoError(t, err)

	nodeStorage := tl.GetBizStatusStorage()
	require.Contains(t, nodeStorage, "test-node")

	// provider should send a start request via tunnel
	recvBizData := <-onBizStartCh
	assert.Equal(t, "test-container", recvBizData.Name)
	assert.Equal(t, "test-namespace/test-pod", recvBizData.PodKey)
	assert.EqualValues(t, model.BizStateResolved, recvBizData.State)
	assert.Equal(t, "mock_resolved", recvBizData.Reason)
	assert.Equal(t, "mock resolved", recvBizData.Message)

	// provider should update the pod status in k8s
	recvK8sData := <-onK8sUpdateCh
	assert.Equal(t, "test-container", recvK8sData.Status.ContainerStatuses[0].Name)
	assert.Equal(t, model.StateReasonAwaitingResync, recvK8sData.Status.ContainerStatuses[0].State.Waiting.Reason)
	assert.Equal(t, "Test resync container", recvK8sData.Status.ContainerStatuses[0].State.Waiting.Message)
	assert.Equal(t, corev1.PodPending, recvK8sData.Status.Phase)
}
