package predicates

import (
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/stretchr/testify/assert"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"testing"
)

func TestVPodPredicate_CreatePass(t *testing.T) {
	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{"suite"})
	predicate := VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
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
	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{"suite"})
	predicate := VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
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
	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{"suite"})
	predicate := VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
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
	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{"suite"})
	predicate := VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
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
	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{"suite"})
	predicate := VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
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
	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{"suite"})
	predicate := VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
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
	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{"suite"})
	predicate := VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
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
	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{"suite"})
	predicate := VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
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
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	predicate := VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
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
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	predicate := VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
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
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	predicate := VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
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
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	predicate := VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
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
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	predicate := VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
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
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	predicate := VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
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
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	predicate := VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
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
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	predicate := VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Generic(event.TypedGenericEvent[*corev1.Node]{
		Object: &corev1.Node{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVNodeLeasePredicate_CreatePass(t *testing.T) {
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})
	predicate := VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Create(event.TypedCreateEvent[*coordinationv1.Lease]{
		Object: &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: model.ComponentVNodeLease},
			},
		},
	})
	assert.True(t, pass)
}

func TestVNodeLeasePredicate_CreateNotPass(t *testing.T) {
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})
	predicate := VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Create(event.TypedCreateEvent[*coordinationv1.Lease]{
		Object: &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVNodeLeasePredicate_UpdatePass(t *testing.T) {
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})
	predicate := VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Update(event.TypedUpdateEvent[*coordinationv1.Lease]{
		ObjectNew: &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: model.ComponentVNodeLease},
			},
		},
	})
	assert.True(t, pass)
}

func TestVNodeLeasePredicate_UpdateNotPass(t *testing.T) {
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})
	predicate := VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Update(event.TypedUpdateEvent[*coordinationv1.Lease]{
		ObjectNew: &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVNodeLeasePredicate_DeletePass(t *testing.T) {
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})
	predicate := VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Delete(event.TypedDeleteEvent[*coordinationv1.Lease]{
		Object: &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: model.ComponentVNodeLease},
			},
		},
	})
	assert.True(t, pass)
}

func TestVNodeLeasePredicate_DeleteNotPass(t *testing.T) {
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})
	predicate := VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Delete(event.TypedDeleteEvent[*coordinationv1.Lease]{
		Object: &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}

func TestVNodeLeasePredicate_GenericPass(t *testing.T) {
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})
	predicate := VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Generic(event.TypedGenericEvent[*coordinationv1.Lease]{
		Object: &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{model.LabelKeyOfComponent: model.ComponentVNodeLease},
			},
		},
	})
	assert.True(t, pass)
}

func TestVNodeLeasePredicate_GenericNotPass(t *testing.T) {
	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})
	predicate := VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeRequirement),
	}
	pass := predicate.Generic(event.TypedGenericEvent[*coordinationv1.Lease]{
		Object: &coordinationv1.Lease{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{},
			},
		},
	})
	assert.False(t, pass)
}
