package predicates

import (
	v1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.TypedPredicate[*v1.Lease] = &VNodeLeasePredicate{}

type VNodeLeasePredicate struct {
	LabelSelector labels.Selector
}

func (V *VNodeLeasePredicate) Create(e event.TypedCreateEvent[*v1.Lease]) bool {
	return V.LabelSelector.Matches(labels.Set(e.Object.Labels))
}

func (V *VNodeLeasePredicate) Delete(e event.TypedDeleteEvent[*v1.Lease]) bool {
	return V.LabelSelector.Matches(labels.Set(e.Object.Labels))
}

func (V *VNodeLeasePredicate) Update(e event.TypedUpdateEvent[*v1.Lease]) bool {
	return V.LabelSelector.Matches(labels.Set(e.ObjectNew.Labels))
}

func (V *VNodeLeasePredicate) Generic(e event.TypedGenericEvent[*v1.Lease]) bool {
	return V.LabelSelector.Matches(labels.Set(e.Object.Labels))
}
