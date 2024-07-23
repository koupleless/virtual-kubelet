package baseRegisterController

import (
	"github.com/koupleless/virtual-kubelet/common/utils"
	corev1 "k8s.io/api/core/v1"
	"strings"
	"time"
)

func getBaseIDFromTopic(topic string) string {
	fileds := strings.Split(topic, "/")
	if len(fileds) < 2 {
		return ""
	}
	if fileds[0] != "koupleless" {
		return ""
	}
	return fileds[1]
}

func expired(publishTimestamp int64, maxLiveMilliSec int64) bool {
	return publishTimestamp+maxLiveMilliSec <= time.Now().UnixMilli()
}

func deleteGraceTimeEqual(old, new *int64) bool {
	if old == nil && new == nil {
		return true
	}
	if old != nil && new != nil {
		return *old == *new
	}
	return false
}

// podShouldEnqueue checks if two pods equal according to podsEqual func and DeleteTimeStamp
func podShouldEnqueue(oldPod, newPod *corev1.Pod) bool {
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
