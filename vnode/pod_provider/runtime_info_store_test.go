package pod_provider

import (
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestRuntimeInfoStore_PutPod(t *testing.T) {
	store := NewRuntimeInfoStore()
	mc := &tunnel.MockTunnel{}
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
	}, mc)
	assert.NotNil(t, store.podKeyToPod["ns1/pod1"])
}

func TestRuntimeInfoStore_DeletePod(t *testing.T) {
	store := NewRuntimeInfoStore()
	mc := &tunnel.MockTunnel{}
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
	}, mc)
	store.DeletePod("ns1/pod1", mc)
	assert.Nil(t, store.podKeyToPod["ns1/pod1"])
}

func TestRuntimeInfoStore_GetPodByKey(t *testing.T) {
	store := NewRuntimeInfoStore()
	mc := &tunnel.MockTunnel{}
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
	}, mc)
	p := store.GetPodByKey("ns1/pod1")
	assert.NotNil(t, p)
}

func TestRuntimeInfoStore_GetPods(t *testing.T) {
	store := NewRuntimeInfoStore()
	mc := &tunnel.MockTunnel{}
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
	}, mc)
	ps := store.GetPods()
	assert.Len(t, ps, 1)
}
