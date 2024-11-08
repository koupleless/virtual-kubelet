package utils

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// DefaultRateLimiter calculates the duration for retry based on the number of retries.
func DefaultRateLimiter(retryTimes int) time.Duration {
	if retryTimes < 30 {
		return time.Duration(retryTimes) * 100 * time.Millisecond
	} else if retryTimes < 100 {
		return time.Duration(retryTimes) * time.Duration(retryTimes) * 100 * time.Millisecond
	} else {
		return 1000 * time.Second
	}
}

// TimedTaskWithInterval runs a task at a specified interval until the context is cancelled.
func TimedTaskWithInterval(ctx context.Context, interval time.Duration, task func(context.Context)) {
	wait.UntilWithContext(ctx, task, interval)
}

// CheckAndFinallyCall checks a condition at a specified interval until it's true or a timeout occurs.
func CheckAndFinallyCall(ctx context.Context, checkFunc func() bool, timeout, interval time.Duration, finally, timeoutCall func()) {
	checkTicker := time.NewTicker(interval)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			logrus.Info("Check and Finally call timeout")
			timeoutCall()
			return
		case <-checkTicker.C:
			if checkFunc() {
				finally()
				return
			}
		}
	}
}

// CallWithRetry attempts to call a function with retries based on a custom rate limiter.
func CallWithRetry(ctx context.Context, call func(retryTimes int) (shouldRetry bool, err error), retryRateLimiter func(retryTimes int) time.Duration) error {
	logger := log.G(ctx)
	retryTimes := 0
	shouldRetry, err := call(retryTimes)
	if retryRateLimiter == nil {
		retryRateLimiter = DefaultRateLimiter
	}

	defer func() {
		if err != nil {
			logger.WithError(err).Infof("Calling with retry failed")
		}
	}()

	for shouldRetry {
		retryTimes++
		timer := time.NewTimer(retryRateLimiter(retryTimes))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			logger.WithError(err).Infof("Calling with retry times: %d", retryTimes)
			shouldRetry, err = call(retryTimes)
		}
	}

	return err
}

// ConvertByteNumToResourceQuantity converts a byte number to a resource quantity.
func ConvertByteNumToResourceQuantity(byteNum int64) resource.Quantity {
	resourceStr := ""
	byteNum /= 1024
	if byteNum <= 0 {
		byteNum = 0
	}
	resourceStr = fmt.Sprintf("%dKi", byteNum)
	return resource.MustParse(resourceStr)
}

// GetEnv retrieves an environment variable or returns a default value if not set.
func GetEnv(key, defaultValue string) string {
	value, found := os.LookupEnv(key)
	if found {
		return value
	}
	return defaultValue
}

// GetPodKey constructs a pod key from a pod object.
func GetPodKey(pod *corev1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

// GetPodKeyFromContainerKey extracts the pod key from a container key.
func GetPodKeyFromContainerKey(containerKey string) string {
	return strings.Join(strings.Split(containerKey, "/")[:2], "/")
}

// GetContainerNameFromContainerKey extracts the container name from a container key.
func GetContainerNameFromContainerKey(containerKey string) string {
	return strings.Join(strings.Split(containerKey, "/")[2:], "/")
}

// GetContainerKey constructs a container key from a pod key and container name.
func GetContainerKey(podKey, containerName string) string {
	return podKey + "/" + containerName
}

// PodsEqual checks if two pods are equal based on specific fields that can be modified after startup.
func PodsEqual(pod1, pod2 *corev1.Pod) bool {
	// Pod Update Only Permits update of:
	// - `spec.containers[*].image`
	// - `spec.initContainers[*].image`
	// - `spec.activeDeadlineSeconds`
	// - `spec.tolerations` (only additions to existing tolerations)
	// - `objectmeta.labels`
	// - `objectmeta.annotations`
	// - `objectmeta.finalizers`
	// compare the values of the pods to see if the values actually changed

	return cmp.Equal(pod1.Spec.Containers, pod2.Spec.Containers) &&
		cmp.Equal(pod1.Spec.InitContainers, pod2.Spec.InitContainers) &&
		cmp.Equal(pod1.Spec.ActiveDeadlineSeconds, pod2.Spec.ActiveDeadlineSeconds) &&
		cmp.Equal(pod1.Spec.Tolerations, pod2.Spec.Tolerations) &&
		cmp.Equal(pod1.ObjectMeta.Labels, pod2.Labels) &&
		cmp.Equal(pod1.ObjectMeta.Annotations, pod2.Annotations) &&
		cmp.Equal(pod1.ObjectMeta.Finalizers, pod2.Finalizers)
}

// FormatNodeName constructs a node name based on node ID and environment.
func FormatNodeName(nodeID, env string) string {
	return fmt.Sprintf("%s.%s.%s", model.VNodePrefix, nodeID, env)
}

// ExtractNodeIDFromNodeName extracts the node ID from a node name.
func ExtractNodeIDFromNodeName(nodeName string) string {
	split := strings.Split(nodeName, ".")
	if len(split) == 1 {
		return ""
	}
	return strings.Join(split[1:len(split)-1], ".")
}

// ConvertBizStatusToContainerStatus converts tunnel container status to Kubernetes container status.
func ConvertBizStatusToContainerStatus(container corev1.Container, data *model.BizStatusData) corev1.ContainerStatus {
	// Determine if the container has started based on the data state
	started := data != nil && strings.EqualFold(data.State, string(model.BizStateActivated))

	// Initialize the return value with common fields
	ret := corev1.ContainerStatus{
		Name:        container.Name,
		ContainerID: container.Name,
		State:       corev1.ContainerState{},
		Ready:       started,
		Started:     &started,
		Image:       container.Image,
		ImageID:     container.Image,
	}

	// Handle the case where data is nil, indicating the container is pending
	// no biz info yet
	if data == nil || strings.EqualFold(data.State, string(model.BizStateUnResolved)) {
		ret.State = corev1.ContainerState{}
		return ret
	}

	// Handle the case where the container is in the resolved state, indicating it's starting
	if strings.EqualFold(data.State, string(model.BizStateResolved)) ||
		strings.EqualFold(data.State, string(model.BizStateBroken)) ||
		strings.EqualFold(data.State, string(model.BizStateDeactivated)) {
		ret.State.Waiting = &corev1.ContainerStateWaiting{
			Reason:  data.Reason,
			Message: data.Message,
			// Note: This state indicates the container is in the process of starting
		}
		return ret
	}

	// Handle the case where the container is in the activated state, indicating it's running
	if strings.EqualFold(data.State, string(model.BizStateActivated)) {
		ret.State.Running = &corev1.ContainerStateRunning{
			StartedAt: metav1.Time{
				Time: data.ChangeTime,
			},
			// Note: This state indicates the container is currently running
		}
	}

	// Handle the case where the container is in the deactivated state, indicating it's terminated
	if strings.EqualFold(data.State, string(model.BizStateStopped)) {
		ret.State.Terminated = &corev1.ContainerStateTerminated{
			ExitCode: 1,
			Reason:   data.Reason,
			Message:  data.Message,
			FinishedAt: metav1.Time{
				Time: data.ChangeTime,
			},
			// TODO: set the start time of this biz?
			StartedAt: metav1.Time{
				Time: data.ChangeTime,
			},
			// Note: This state indicates the container has been terminated
		}
	}
	return ret
}

// SplitMetaNamespaceKey splits a key into namespace and name.
func SplitMetaNamespaceKey(key string) (namespace, name string, err error) {
	parts := strings.Split(key, "/")
	switch len(parts) {
	case 1:
		// name only, no namespace
		return "", parts[0], nil
	case 2:
		// namespace and name
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("unexpected key format: %q", key)
}

// GetBizVersionFromContainer extracts the biz version from a container's env vars
func getBizVersionFromContainer(container *corev1.Container) string {
	bizVersion := ""
	for _, env := range container.Env {
		if env.Name == "BIZ_VERSION" {
			bizVersion = env.Value
			break
		}
	}
	return bizVersion
}

// GetBizIdentity creates a unique identifier from biz name and version
func getBizIdentity(bizName, bizVersion string) string {
	return bizName + ":" + bizVersion
}

// GetContainerUniqueKey returns a unique key for the container
func GetContainerUniqueKey(container *corev1.Container) string {
	return getBizIdentity(container.Name, getBizVersionFromContainer(container))
}
