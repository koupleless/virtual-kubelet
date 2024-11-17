package provider

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestVPodStore_PutPod(t *testing.T) {
	store := NewVPodStore()
	store.PutPod(&corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod1",
			Namespace: "ns1",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "container1",
					Image: "image1",
				},
			},
		},
	})
	assert.NotNil(t, store.podKeyToPod["ns1/pod1"])
}

func TestVPodStore_DeletePod(t *testing.T) {
	store := NewVPodStore()
	store.PutPod(&corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod1",
			Namespace: "ns1",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "container1",
					Image: "image1",
				},
			},
		},
	})
	store.DeletePod("ns1/pod1")
	assert.Nil(t, store.podKeyToPod["ns1/pod1"])
}

func TestVPodStore_GetPodByKey(t *testing.T) {
	store := NewVPodStore()
	store.PutPod(&corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod1",
			Namespace: "ns1",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "container1",
					Image: "image1",
				},
			},
		},
	})
	p := store.GetPodByKey("ns1/pod1")
	assert.NotNil(t, p)
}

func TestVPodStore_GetPods(t *testing.T) {
	store := NewVPodStore()
	store.PutPod(&corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:      "pod1",
			Namespace: "ns1",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "container1",
					Image: "image1",
				},
			},
		},
	})
	ps := store.GetPods()
	assert.Len(t, ps, 1)
}
