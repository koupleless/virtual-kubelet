package pod_provider

import (
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func TestRuntimeInfoStore_PutPod(t *testing.T) {
	store := NewRuntimeInfoStore()
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

func TestRuntimeInfoStore_DeletePod(t *testing.T) {
	store := NewRuntimeInfoStore()
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

func TestRuntimeInfoStore_GetRelatedPodKeysByContainerName(t *testing.T) {
	store := NewRuntimeInfoStore()
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
	podKeys := store.GetRelatedPodKeysByContainerName("container1")
	assert.Len(t, podKeys, 1)
}

func TestRuntimeInfoStore_GetContainer(t *testing.T) {
	store := NewRuntimeInfoStore()
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
	c := store.GetContainer("ns1/pod1/container1")
	assert.NotNil(t, c)
}

func TestRuntimeInfoStore_GetPodByKey(t *testing.T) {
	store := NewRuntimeInfoStore()
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

func TestRuntimeInfoStore_GetPods(t *testing.T) {
	store := NewRuntimeInfoStore()
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

func TestRuntimeInfoStore_PutContainerInfo(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutContainerInfo(model.ContainerStatusData{
		Key:        "suite-container",
		Name:       "container",
		PodKey:     "suite",
		State:      model.ContainerStateActivated,
		ChangeTime: time.Now(),
		Reason:     "suite-reason",
		Message:    "suite-message",
	})
	assert.NotNil(t, store.latestContainerInfosFromNode["suite-container"])
}

func TestRuntimeInfoStore_GetLatestContainerInfos(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutContainerInfo(model.ContainerStatusData{
		Key:        "suite-container",
		Name:       "container",
		PodKey:     "suite",
		State:      model.ContainerStateActivated,
		ChangeTime: time.Now(),
		Reason:     "suite-reason",
		Message:    "suite-message",
	})
	infos := store.GetLatestContainerInfos()
	assert.Len(t, infos, 1)
}

func TestRuntimeInfoStore_GetLatestContainerInfoByContainerKey(t *testing.T) {
	store := NewRuntimeInfoStore()
	store.PutContainerInfo(model.ContainerStatusData{
		Key:        "suite-container",
		Name:       "container",
		PodKey:     "suite",
		State:      model.ContainerStateActivated,
		ChangeTime: time.Now(),
		Reason:     "suite-reason",
		Message:    "suite-message",
	})
	c := store.GetLatestContainerInfoByContainerKey("suite-container")
	assert.NotNil(t, c)
}
