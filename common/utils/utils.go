package utils

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/log"
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
func CheckAndFinallyCall(ctx context.Context, checkFunc func() (bool, error), timeout, interval time.Duration, finally, timeoutCall func()) {
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
			// TODO: handle the error
			finished, err := checkFunc()
			if err != nil {
				return
			}
			if finished {
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

func NodeStatusEqual(status1, status2 model.NodeStatusData) bool {
	return cmp.Equal(status1.Resources, status2.Resources) &&
		cmp.Equal(status1.CustomConditions, status2.CustomConditions) &&
		cmp.Equal(status1.CustomAnnotations, status2.CustomAnnotations) &&
		cmp.Equal(status1.CustomLabels, status2.CustomLabels)
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

// MergeNodeFromProvider constructs a virtual node based on the latest node status data.
func MergeNodeFromProvider(node *corev1.Node, data model.NodeStatusData) *corev1.Node {
	vnodeCopy := node.DeepCopy() // Create a deep copy of the node info.
	// Set the node status to running.
	vnodeCopy.Status.Phase = corev1.NodeRunning
	// Initialize a map to hold node conditions.
	conditionMap := map[corev1.NodeConditionType]corev1.NodeCondition{
		corev1.NodeReady: {
			Type:   corev1.NodeReady,
			Status: corev1.ConditionTrue,
		},
		corev1.NodeMemoryPressure: {
			Type:   corev1.NodeMemoryPressure,
			Status: corev1.ConditionFalse,
		},
		corev1.NodeDiskPressure: {
			Type:   corev1.NodeDiskPressure,
			Status: corev1.ConditionFalse,
		},
		corev1.NodePIDPressure: {
			Type:   corev1.NodePIDPressure,
			Status: corev1.ConditionFalse,
		},
		corev1.NodeNetworkUnavailable: {
			Type:   corev1.NodeNetworkUnavailable,
			Status: corev1.ConditionFalse,
		},
	}

	// Add custom conditions to the condition map.
	for _, customCondition := range data.CustomConditions {
		conditionMap[customCondition.Type] = customCondition
	}

	// Convert the condition map to a slice of conditions.
	conditions := make([]corev1.NodeCondition, 0)
	for _, condition := range conditionMap {
		conditions = append(conditions, condition)
	}
	vnodeCopy.Status.Conditions = conditions // Set the conditions on the vnode copy.
	if vnodeCopy.Annotations == nil {
		vnodeCopy.Annotations = make(map[string]string)
	}
	if vnodeCopy.Labels == nil {
		vnodeCopy.Labels = make(map[string]string)
	}
	// Set custom annotations.
	for key, value := range data.CustomAnnotations {
		vnodeCopy.Annotations[key] = value
	}
	// Set custom labels.
	for key, value := range data.CustomLabels {
		vnodeCopy.Labels[key] = value
	}
	// Set resource capacities and allocatable amounts.
	if vnodeCopy.Status.Capacity == nil {
		vnodeCopy.Status.Capacity = make(corev1.ResourceList)
	}
	if vnodeCopy.Status.Allocatable == nil {
		vnodeCopy.Status.Allocatable = make(corev1.ResourceList)
	}
	for resourceName, status := range data.Resources {
		vnodeCopy.Status.Capacity[resourceName] = status.Capacity
		vnodeCopy.Status.Allocatable[resourceName] = status.Allocatable
	}
	return vnodeCopy // Return the constructed vnode.
}

// ConvertBizStatusToContainerStatus converts tunnel container status to Kubernetes container status, if not the status for the container, then create a empty state container status
func ConvertBizStatusToContainerStatus(container *corev1.Container, containerStatus *corev1.ContainerStatus, data *model.BizStatusData) (*corev1.ContainerStatus, error) {
	// this may be a little complex to handle the case that parameters is not nil or for same container name
	if containerStatus == nil {
		if data == nil || (container.Name != data.Name) {
			started := false
			return &corev1.ContainerStatus{
				Name:        container.Name,
				ContainerID: container.Name,
				State:       corev1.ContainerState{},
				Ready:       started,
				Started:     &started,
				Image:       container.Image,
				ImageID:     container.Image,
			}, nil
		}
	}

	if data == nil {
		// reuse the old status, cause the new data is nil
		if containerStatus.Name == container.Name {
			return containerStatus, nil
		} else {
			// can't handle this case
			return nil, fmt.Errorf("convert biz status to container status but container name mismatch: %s != %s", containerStatus.Name, container.Name)
		}
	}

	if container.Name != data.Name {
		// reuse the old status, cause the new data is not for this container
		if containerStatus.Name == container.Name {
			return containerStatus, nil
		} else {
			return nil, fmt.Errorf("convert biz status to container status but container name mismatch: %s != %s", container.Name, data.Name)
		}
	}

	// Determine if the container has started based on the data state
	started := strings.EqualFold(data.State, string(model.BizStateActivated))

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

	if strings.EqualFold(data.State, string(model.BizStateUnResolved)) {
		// Handle the case where data is nil, indicating the container is pending
		// no biz info yet
		ret.State = corev1.ContainerState{}
		return &ret, nil
	} else if strings.EqualFold(data.State, string(model.BizStateResolved)) ||
		strings.EqualFold(data.State, string(model.BizStateBroken)) ||
		strings.EqualFold(data.State, string(model.BizStateDeactivated)) {
		// Handle the case where the container is in the resolved state, indicating it's starting
		ret.State.Waiting = &corev1.ContainerStateWaiting{
			Reason:  data.Reason,
			Message: data.Message,
			// Note: This state indicates the container is in the process of starting
		}
		return &ret, nil
	} else if strings.EqualFold(data.State, string(model.BizStateActivated)) {
		// Handle the case where the container is in the activated state, indicating it's running
		ret.State.Running = &corev1.ContainerStateRunning{
			StartedAt: metav1.Time{
				Time: data.ChangeTime,
			},
			// Note: This state indicates the container is currently running
		}
	} else if strings.EqualFold(data.State, string(model.BizStateStopped)) {
		// Handle the case where the container is in the deactivated state, indicating it's terminated
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

	return &ret, nil
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

// GetBizUniqueKey returns a unique key for the container
func GetBizUniqueKey(container *corev1.Container) string {
	return getBizIdentity(container.Name, getBizVersionFromContainer(container))
}

func FillPodKey(pods []corev1.Pod, bizStatusDatas []model.BizStatusData) (toUpdate []model.BizStatusData, toDelete []model.BizStatusData) {
	bizKeyToPodKey := make(map[string]string)
	// 一个 vnode 上,  所有的 biz container name 是唯一的
	for _, pod := range pods {
		for _, container := range pod.Spec.Containers {
			if strings.Contains(container.Image, ".jar") {
				bizKeyToPodKey[GetBizUniqueKey(&container)] = GetPodKey(&pod)
			}
		}
	}

	// if bizStatusData.PodKey is empty, try to find it from bizKeyToPodKey
	bizStatusDatasWithPodKey := make([]model.BizStatusData, 0, len(bizStatusDatas))
	bizStatusDatasWithNoPodKey := make([]model.BizStatusData, 0, len(bizStatusDatas))
	for i, _ := range bizStatusDatas {
		if podKey, ok := bizKeyToPodKey[bizStatusDatas[i].Key]; ok && bizStatusDatas[i].PodKey == "" {
			bizStatusDatas[i].PodKey = podKey
		}
		if bizStatusDatas[i].PodKey != "" {
			bizStatusDatasWithPodKey = append(bizStatusDatasWithPodKey, bizStatusDatas[i])
		} else {
			bizStatusDatasWithNoPodKey = append(bizStatusDatasWithNoPodKey, bizStatusDatas[i])
			log.G(context.Background()).Infof("biz container %s in k8s not found, try to uninstall.", bizStatusDatas[i].Key)
		}
	}

	return bizStatusDatasWithPodKey, bizStatusDatasWithNoPodKey
}
