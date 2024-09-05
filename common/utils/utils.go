package utils

import (
	"context"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strings"
	"time"
)

func TimedTaskWithInterval(ctx context.Context, interval time.Duration, task func(context.Context)) {
	task(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			task(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func CheckAndFinallyCall(checkFunc func() bool, timeout, interval time.Duration, finally, timeoutCall func()) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	checkTicker := time.NewTicker(interval)
	for range checkTicker.C {
		select {
		case <-ctx.Done():
			// TODO timeout log
			logrus.Info("Check and Finally call timeout")
			timeoutCall()
			return
		default:
		}
		if checkFunc() {
			finally()
			return
		}
	}
}

func ConvertByteNumToResourceQuantity(byteNum int64) resource.Quantity {
	resourceStr := ""
	byteNum /= 1024
	if byteNum <= 0 {
		byteNum = 0
	}
	resourceStr = fmt.Sprintf("%dKi", byteNum)
	return resource.MustParse(resourceStr)
}

func GetEnv(key, defaultValue string) string {
	value, found := os.LookupEnv(key)
	if found {
		return value
	}
	return defaultValue
}

func GetPodKey(pod *corev1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

func GetPodKeyFromContainerKey(containerKey string) string {
	return strings.Join(strings.Split(containerKey, "/")[:2], "/")
}

func GetContainerKey(podKey, containerName string) string {
	return podKey + "/" + containerName
}

// PodsEqual checks if two pods are equal according to the fields we know that are allowed
// to be modified after startup time.
func PodsEqual(pod1, pod2 *corev1.Pod) bool {
	// Pod Update Only Permits update of:
	// - `spec.containers[*].image`
	// - `spec.initContainers[*].image`
	// - `spec.activeDeadlineSeconds`
	// - `spec.tolerations` (only additions to existing tolerations)
	// - `objectmeta.labels`
	// - `objectmeta.annotations`
	// compare the values of the pods to see if the values actually changed

	return cmp.Equal(pod1.Spec.Containers, pod2.Spec.Containers) &&
		cmp.Equal(pod1.Spec.InitContainers, pod2.Spec.InitContainers) &&
		cmp.Equal(pod1.Spec.ActiveDeadlineSeconds, pod2.Spec.ActiveDeadlineSeconds) &&
		cmp.Equal(pod1.Spec.Tolerations, pod2.Spec.Tolerations) &&
		cmp.Equal(pod1.ObjectMeta.Labels, pod2.Labels) &&
		cmp.Equal(pod1.ObjectMeta.Annotations, pod2.Annotations) &&
		cmp.Equal(pod1.ObjectMeta.Finalizers, pod2.Finalizers)
}

func FormatBaseNodeName(baseID string) string {
	return fmt.Sprintf("%s.%s", model.VNodePrefix, baseID)
}

func ExtractBaseIDFromNodeName(nodeName string) string {
	splits := strings.Split(nodeName, ".")
	return splits[len(splits)-1]
}

func TranslateContainerStatusFromTunnelToContainerStatus(container corev1.Container, data *model.ContainerStatusData) corev1.ContainerStatus {
	started :=
		data != nil && data.State == model.ContainerStateActivated

	ret := corev1.ContainerStatus{
		Name:        container.Name,
		ContainerID: container.Name,
		State:       corev1.ContainerState{},
		Ready:       started,
		Started:     &started,
		Image:       container.Image,
		ImageID:     container.Image,
	}

	if data == nil {
		ret.State.Waiting = &corev1.ContainerStateWaiting{
			Reason:  "ContainerPending",
			Message: "Container is waiting for start",
		}
		return ret
	}

	if data.State == model.ContainerStateResolved {
		// starting
		ret.State.Waiting = &corev1.ContainerStateWaiting{
			Reason:  "ContainerResolved",
			Message: "Container is starting but not activated",
		}
		return ret
	}

	if data.State == model.ContainerStateActivated {
		ret.State.Running = &corev1.ContainerStateRunning{
			StartedAt: metav1.Time{
				Time: data.ChangeTime,
			},
		}
	}

	if data.State == model.ContainerStateDeactivated {
		ret.State.Terminated = &corev1.ContainerStateTerminated{
			ExitCode: 1,
			Reason:   data.Reason,
			Message:  data.Message,
			FinishedAt: metav1.Time{
				Time: data.ChangeTime,
			},
		}
	}
	return ret
}
