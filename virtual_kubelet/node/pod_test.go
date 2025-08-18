package node

import (
	"testing"

	"github.com/koupleless/virtual-kubelet/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_shouldReSyncBizContainer(t *testing.T) {
	assert.True(t, shouldReSyncBizContainer(&corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "test",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason: model.StateReasonAwaitingResync,
						},
					},
				},
			},
		},
	}))
	assert.False(t, shouldReSyncBizContainer(&corev1.Pod{}))
}
