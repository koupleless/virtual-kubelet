package vnode_controller

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"testing"
	"time"
)

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
