package predicates

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Summary: This file defines a predicate for filtering corev1.Node events based on label selectors.

var _ predicate.TypedPredicate[*corev1.Node] = &VNodePredicate{}

type VNodePredicate struct {
	VNodeLabelSelector labels.Selector
}

func (V *VNodePredicate) Create(e event.TypedCreateEvent[*corev1.Node]) bool {
	return V.VNodeLabelSelector.Matches(labels.Set(e.Object.Labels))
}

func (V *VNodePredicate) Delete(e event.TypedDeleteEvent[*corev1.Node]) bool {
	return V.VNodeLabelSelector.Matches(labels.Set(e.Object.Labels))
}

func (V *VNodePredicate) Update(e event.TypedUpdateEvent[*corev1.Node]) bool {
	return V.VNodeLabelSelector.Matches(labels.Set(e.ObjectNew.Labels))
}

func (V *VNodePredicate) Generic(e event.TypedGenericEvent[*corev1.Node]) bool {
	return V.VNodeLabelSelector.Matches(labels.Set(e.Object.Labels))
}
