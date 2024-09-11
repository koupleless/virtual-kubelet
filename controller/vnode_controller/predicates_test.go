package vnode_controller

import (
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"testing"
)

func TestVPodPredicate_CreatePass(t *testing.T) {
	predicate := VPodPredicate{
		VPodIdentity: "suite",
	}
	pass := predicate.Create(event.TypedCreateEvent[*corev1.Pod]{
		Object: &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: "suite"},
			},
			Spec: corev1.PodSpec{
				NodeName: "test",
			},
		},
	})
	assert.True(t, pass)
}

func TestVPodPredicate_CreateNotPass(t *testing.T) {
	predicate := VPodPredicate{
		VPodIdentity: "suite",
	}
	pass := predicate.Create(event.TypedCreateEvent[*corev1.Pod]{
		Object: &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVPodPredicate_UpdatePass(t *testing.T) {
	predicate := VPodPredicate{
		VPodIdentity: "suite",
	}
	pass := predicate.Update(event.TypedUpdateEvent[*corev1.Pod]{
		ObjectNew: &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: "suite"},
			},
			Spec: corev1.PodSpec{
				NodeName: "test",
			},
		},
	})
	assert.True(t, pass)
}

func TestVPodPredicate_UpdateNotPass(t *testing.T) {
	predicate := VPodPredicate{
		VPodIdentity: "suite",
	}
	pass := predicate.Update(event.TypedUpdateEvent[*corev1.Pod]{
		ObjectNew: &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVPodPredicate_DeletePass(t *testing.T) {
	predicate := VPodPredicate{
		VPodIdentity: "suite",
	}
	pass := predicate.Delete(event.TypedDeleteEvent[*corev1.Pod]{
		Object: &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: "suite"},
			}, Spec: corev1.PodSpec{
				NodeName: "test",
			},
		},
	})
	assert.True(t, pass)
}

func TestVPodPredicate_DeleteNotPass(t *testing.T) {
	predicate := VPodPredicate{
		VPodIdentity: "suite",
	}
	pass := predicate.Delete(event.TypedDeleteEvent[*corev1.Pod]{
		Object: &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVPodPredicate_GenericPass(t *testing.T) {
	predicate := VPodPredicate{
		VPodIdentity: "suite",
	}
	pass := predicate.Generic(event.TypedGenericEvent[*corev1.Pod]{
		Object: &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: "suite"},
			},
			Spec: corev1.PodSpec{
				NodeName: "test",
			},
		},
	})
	assert.True(t, pass)
}

func TestVPodPredicate_GenericNotPass(t *testing.T) {
	predicate := VPodPredicate{
		VPodIdentity: "suite",
	}
	pass := predicate.Generic(event.TypedGenericEvent[*corev1.Pod]{
		Object: &corev1.Pod{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVNodePredicate_CreatePass(t *testing.T) {
	predicate := VNodePredicate{}
	pass := predicate.Create(event.TypedCreateEvent[*corev1.Node]{
		Object: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: model.ComponentVNode},
			},
		},
	})
	assert.True(t, pass)
}

func TestVNodePredicate_CreateNotPass(t *testing.T) {
	predicate := VNodePredicate{}
	pass := predicate.Create(event.TypedCreateEvent[*corev1.Node]{
		Object: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVNodePredicate_UpdatePass(t *testing.T) {
	predicate := VNodePredicate{}
	pass := predicate.Update(event.TypedUpdateEvent[*corev1.Node]{
		ObjectNew: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: model.ComponentVNode},
			},
		},
	})
	assert.True(t, pass)
}

func TestVNodePredicate_UpdateNotPass(t *testing.T) {
	predicate := VNodePredicate{}
	pass := predicate.Update(event.TypedUpdateEvent[*corev1.Node]{
		ObjectNew: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVNodePredicate_DeletePass(t *testing.T) {
	predicate := VNodePredicate{}
	pass := predicate.Delete(event.TypedDeleteEvent[*corev1.Node]{
		Object: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: model.ComponentVNode},
			},
		},
	})
	assert.True(t, pass)
}

func TestVNodePredicate_DeleteNotPass(t *testing.T) {
	predicate := VNodePredicate{}
	pass := predicate.Delete(event.TypedDeleteEvent[*corev1.Node]{
		Object: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVNodePredicate_GenericPass(t *testing.T) {
	predicate := VNodePredicate{}
	pass := predicate.Generic(event.TypedGenericEvent[*corev1.Node]{
		Object: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: model.ComponentVNode},
			},
		},
	})
	assert.True(t, pass)
}

func TestVNodePredicate_GenericNotPass(t *testing.T) {
	predicate := VNodePredicate{}
	pass := predicate.Generic(event.TypedGenericEvent[*corev1.Node]{
		Object: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}
