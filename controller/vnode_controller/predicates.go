package vnode_controller

import (
	"github.com/koupleless/virtual-kubelet/model"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type VPodPredicate struct {
	VPodIdentity string
}

func (V *VPodPredicate) checkIsVPod(componentKey string) bool {
	return componentKey == V.VPodIdentity
}

func (V *VPodPredicate) Create(e event.TypedCreateEvent[*corev1.Pod]) bool {
	return V.checkIsVPod(e.Object.Labels[model.LabelKeyOfComponent])
}

func (V *VPodPredicate) Delete(e event.TypedDeleteEvent[*corev1.Pod]) bool {
	return V.checkIsVPod(e.Object.Labels[model.LabelKeyOfComponent])
}

func (V *VPodPredicate) Update(e event.TypedUpdateEvent[*corev1.Pod]) bool {
	return V.checkIsVPod(e.ObjectNew.Labels[model.LabelKeyOfComponent])
}

func (V *VPodPredicate) Generic(e event.TypedGenericEvent[*corev1.Pod]) bool {
	return V.checkIsVPod(e.Object.Labels[model.LabelKeyOfComponent])
}

type VNodePredicate struct{}

func (V *VNodePredicate) checkIsVNode(componentKey string) bool {
	return componentKey == model.ComponentVNode
}

func (V *VNodePredicate) Create(e event.TypedCreateEvent[*corev1.Node]) bool {
	return V.checkIsVNode(e.Object.Labels[model.LabelKeyOfComponent])
}

func (V *VNodePredicate) Delete(e event.TypedDeleteEvent[*corev1.Node]) bool {
	return V.checkIsVNode(e.Object.Labels[model.LabelKeyOfComponent])
}

func (V *VNodePredicate) Update(e event.TypedUpdateEvent[*corev1.Node]) bool {
	return V.checkIsVNode(e.ObjectNew.Labels[model.LabelKeyOfComponent])
}

func (V *VNodePredicate) Generic(e event.TypedGenericEvent[*corev1.Node]) bool {
	return V.checkIsVNode(e.Object.Labels[model.LabelKeyOfComponent])
}

var _ predicate.TypedPredicate[*corev1.Pod] = &VPodPredicate{}
var _ predicate.TypedPredicate[*corev1.Node] = &VNodePredicate{}
