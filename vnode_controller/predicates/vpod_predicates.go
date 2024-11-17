package predicates

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Summary: This file defines a predicate for filtering corev1.Pod events based on label selectors.

var _ predicate.TypedPredicate[*corev1.Pod] = &VPodPredicate{}

type VPodPredicate struct {
	VPodLabelSelector labels.Selector
}

func (V *VPodPredicate) Create(e event.TypedCreateEvent[*corev1.Pod]) bool {
	return V.VPodLabelSelector.Matches(labels.Set(e.Object.Labels)) && e.Object.Spec.NodeName != ""
}

func (V *VPodPredicate) Delete(e event.TypedDeleteEvent[*corev1.Pod]) bool {
	return V.VPodLabelSelector.Matches(labels.Set(e.Object.Labels)) && e.Object.Spec.NodeName != ""
}

func (V *VPodPredicate) Update(e event.TypedUpdateEvent[*corev1.Pod]) bool {
	return V.VPodLabelSelector.Matches(labels.Set(e.ObjectNew.Labels)) && e.ObjectNew.Spec.NodeName != ""
}

func (V *VPodPredicate) Generic(e event.TypedGenericEvent[*corev1.Pod]) bool {
	return V.VPodLabelSelector.Matches(labels.Set(e.Object.Labels)) && e.Object.Spec.NodeName != ""
}
