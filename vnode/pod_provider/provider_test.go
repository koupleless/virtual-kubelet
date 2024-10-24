package pod_provider

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestSyncRelatedPodStatus(t *testing.T) {
	provider := NewVPodProvider("default", "127.0.0.1", "123", nil, &tunnel.MockTunnel{})
	provider.syncRelatedPodStatus(context.TODO(), "default", "test")
	provider.runtimeInfoStore.containerUniqueKeyKeyToRelatedPodKey["test"] = map[string]bool{
		"test": true,
	}
	provider.syncRelatedPodStatus(context.TODO(), model.PodKeyAll, "test")
}

func TestSyncAllContainerInfo(t *testing.T) {
	tl := &tunnel.MockTunnel{}
	provider := NewVPodProvider("default", "127.0.0.1", "123", nil, tl)
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
	provider.runtimeInfoStore.podKeyToPod = map[string]*corev1.Pod{
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
	provider.SyncAllContainerInfo(context.TODO(), []model.ContainerStatusData{
		{
			Key: tl.GetContainerUniqueKey(utils.GetPodKey(pod), &corev1.Container{
				Name: "test-container",
			}),
		},
	})
}

func TestUpdateDeletedPod(t *testing.T) {
	tl := &tunnel.MockTunnel{}
	provider := NewVPodProvider("default", "127.0.0.1", "123", nil, tl)
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
