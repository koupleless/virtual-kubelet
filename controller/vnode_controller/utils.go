package vnode_controller

import (
	"github.com/koupleless/virtual-kubelet/common/utils"
	corev1 "k8s.io/api/core/v1"
)

// deleteGraceTimeEqual: This function checks if two int64 pointers are equal.
// Parameters:
// - old: The old int64 pointer.
// - new: The new int64 pointer.
// Returns: A boolean value indicating if the two int64 pointers are equal.
func deleteGraceTimeEqual(old, new *int64) bool {
	if old == nil && new == nil {
		return true
	}
	if old != nil && new != nil {
		return *old == *new
	}
	return false
}

// podShouldEnqueue: This function checks if two pods are equal according to the podsEqual function and the DeletionTimeStamp.
// Parameters:
// - oldPod: The old pod.
// - newPod: The new pod.
// Returns: A boolean value indicating if the two pods are equal.
func podShouldEnqueue(oldPod, newPod *corev1.Pod) bool {
	if oldPod == nil || newPod == nil {
		return false
	}
	if !utils.PodsEqual(oldPod, newPod) {
		return true
	}
	if !deleteGraceTimeEqual(oldPod.DeletionGracePeriodSeconds, newPod.DeletionGracePeriodSeconds) {
		return true
	}
	if !oldPod.DeletionTimestamp.Equal(newPod.DeletionTimestamp) {
		return true
	}
	return false
}
