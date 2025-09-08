package provider

import (
	"context"
	"testing"
	"time"

	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestUpdatePod(t *testing.T) {
	tl := &tunnel.MockTunnel{}
	_ = tl.Start("test", "test")
	tl.RegisterCallback(
		func(info model.NodeInfo) {},
		func(s string, data model.NodeStatusData) {},
		func(s string, data []model.BizStatusData) {},
		func(s string, data model.BizStatusData) {},
	)
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test",
			Namespace:         "default",
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

	fakeCli := fake.NewFakeClient(oldPod)

	provider := NewVPodProvider("default", "127.0.0.1", "123", fakeCli, nil, tl)

	newPodCh := make(chan *corev1.Pod, 1)
	provider.NotifyPods(context.Background(), func(pod *corev1.Pod) {
		newPodCh <- pod
	})
	provider.vPodStore.podKeyToPod = map[string]*corev1.Pod{
		"default/test": oldPod,
	}

	podToUpdate := oldPod.DeepCopy()
	podToUpdate.Spec.Containers = append(
		podToUpdate.Spec.Containers,
		corev1.Container{
			Name:  "test-container-2",
			Image: "test-image-2",
		})

	err := provider.UpdatePod(context.TODO(), podToUpdate)
	assert.NoError(t, err)

	revdPod := <-newPodCh
	assert.Len(t, revdPod.Spec.Containers, 2)
}
