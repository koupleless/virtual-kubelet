/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package pod_provider

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/koupleless/virtual-kubelet/common/tracker"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koupleless/virtual-kubelet/common/log"
	corev1 "k8s.io/api/core/v1"
)

// Define the VPodProvider struct
var _ nodeutil.Provider = &VPodProvider{}
var _ virtual_kubelet.PodNotifier = &VPodProvider{}

// VPodProvider is a struct that implements the nodeutil.Provider and virtual_kubelet.PodNotifier interfaces
type VPodProvider struct {
	Namespace        string
	nodeID           string
	localIP          string
	client           client.Client
	runtimeInfoStore *RuntimeInfoStore

	tunnel tunnel.Tunnel

	port int

	notify func(pod *corev1.Pod)
}

// NotifyPods is a method of VPodProvider that sets the notify function
func (b *VPodProvider) NotifyPods(_ context.Context, cb func(*corev1.Pod)) {
	b.notify = cb
}

// NewVPodProvider is a function that creates a new VPodProvider instance
func NewVPodProvider(namespace, localIP, nodeID string, client client.Client, t tunnel.Tunnel) *VPodProvider {
	provider := &VPodProvider{
		Namespace:        namespace,
		localIP:          localIP,
		nodeID:           nodeID,
		client:           client,
		tunnel:           t,
		runtimeInfoStore: NewRuntimeInfoStore(),
	}

	return provider
}

// syncRelatedPodStatus is a method of VPodProvider that synchronizes the status of related pods
func (b *VPodProvider) syncRelatedPodStatus(ctx context.Context, bizStatusData model.BizStatusData) {
	logger := log.G(ctx)
	pod := b.runtimeInfoStore.GetPodByKey(bizStatusData.PodKey)
	if pod == nil {
		logger.Errorf("skip updating non-exist pod status for biz %s pod %s", bizStatusData.Key, bizStatusData.PodKey)
		return
	}
	b.updatePodStatusToKubernetes(ctx, pod, bizStatusData)
}

// updatePodStatusToKubernetes is a method of VPodProvider that updates the status of a pod to Kubernetes
func (b *VPodProvider) updatePodStatusToKubernetes(ctx context.Context, pod *corev1.Pod, bizStatus model.BizStatusData) {
	// merge pod status from k8s and biz status
	podStatus, _ := b.GetPodStatus(ctx, pod, bizStatus)

	podCopy := pod.DeepCopy()
	podStatus.DeepCopyInto(&podCopy.Status)
	b.runtimeInfoStore.PutPod(podCopy, b.tunnel)
	b.notify(podCopy)
}

// SyncAllContainerInfo is a method of VPodProvider that synchronizes the information of all containers
func (b *VPodProvider) SyncAllContainerInfo(ctx context.Context, bizStatusDatas []model.BizStatusData) {
	bizKeyToBizStatusData := make(map[string]model.BizStatusData)
	for _, bizStatusData := range bizStatusDatas {
		bizKeyToBizStatusData[bizStatusData.Key] = bizStatusData
	}

	pods := b.runtimeInfoStore.GetPods()
	// sort by create time
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].CreationTimestamp.UnixMilli() > pods[j].CreationTimestamp.UnixMilli()
	})

	// Initialize an empty slice to store updated container information
	toUpdateBizStatusDatas := make([]model.BizStatusData, 0)
	// Get the current time to use for change time
	now := time.Now()
	// Iterate through each pod
	for _, pod := range pods {
		// Get the key of the pod
		podKey := utils.GetPodKey(pod)
		// Iterate through each container in the pod
		for _, container := range pod.Spec.Containers {
			// Get the unique key of the container
			bizKey := utils.GetBizUniqueKey(&container)
			// Check if container information exists for the container key
			bizStatusData, has := bizKeyToBizStatusData[bizKey]
			// If container information does not exist, create a new deactivated instance
			if !has {
				bizStatusData = model.BizStatusData{
					Key:        bizKey,
					Name:       container.Name,
					PodKey:     podKey,
					State:      string(model.BizStateUnResolved),
					ChangeTime: now,
				}
			}
			// Attempt to update the container status
			toUpdate := b.runtimeInfoStore.CheckContainerStatusNeedSync(bizStatusData)
			// If the update was successful, add the container information to the updated list
			if toUpdate {
				toUpdateBizStatusDatas = append(toUpdateBizStatusDatas, bizStatusData)
			}

			log.G(ctx).Infof("container %s/%s need update: %s", podKey, bizKey, toUpdate)
		}
	}

	// Iterate through the provided container information and sync the related pod status
	for _, toUpdateBizStatus := range toUpdateBizStatusDatas {
		b.syncRelatedPodStatus(ctx, toUpdateBizStatus)
	}
}

// SyncOneContainerInfo is a method of VPodProvider that synchronizes the information of a single container
func (b *VPodProvider) SyncOneContainerInfo(ctx context.Context, bizStatusData model.BizStatusData) {
	needSync := b.runtimeInfoStore.CheckContainerStatusNeedSync(bizStatusData)
	if needSync {
		// only when container status updated, update related pod status
		b.syncRelatedPodStatus(ctx, bizStatusData)
	}
}

// startContainer is a method of VPodProvider that starts a container
func (b *VPodProvider) startContainer(ctx context.Context, podKey string, container *corev1.Container) error {
	// clear local container status cache
	return b.tunnel.StartContainer(ctx, b.nodeID, podKey, container)
}

// stopContainer is a method of VPodProvider that stops a container
func (b *VPodProvider) stopContainer(ctx context.Context, podKey string, container *corev1.Container) error {
	return b.tunnel.ShutdownContainer(ctx, b.nodeID, podKey, container)
}

// handleContainerStart is a method of VPodProvider that handles the start of a container
func (b *VPodProvider) handleContainerStart(ctx context.Context, pod *corev1.Pod, containers []corev1.Container) {
	podKey := utils.GetPodKey(pod)

	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("HandleContainerStartOperation")

	labelMap := pod.Labels
	if labelMap == nil {
		labelMap = make(map[string]string)
	}

	for _, container := range containers {
		err := tracker.G().FuncTrack(labelMap[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventContainerStart, labelMap, func() (error, model.ErrorCode) {
			err := utils.CallWithRetry(ctx, func(_ int) (bool, error) {
				innerErr := b.startContainer(ctx, podKey, &container)

				return innerErr != nil, innerErr
			}, nil)
			if err != nil {
				return err, model.CodeContainerStartFailed
			}
			return nil, model.CodeSuccess
		})
		if err != nil {
			logger.WithError(err).WithField("containerKey", utils.GetContainerKey(podKey, container.Name)).Error("ContainerStartFailed")
		}
	}
}

// handleContainerShutdown is a method of VPodProvider that handles the shutdown of a container
func (b *VPodProvider) handleContainerShutdown(ctx context.Context, pod *corev1.Pod, containers []corev1.Container) {
	podKey := utils.GetPodKey(pod)

	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("HandleContainerShutdownOperation")

	labelMap := pod.Labels
	if labelMap == nil {
		labelMap = make(map[string]string)
	}

	for _, container := range containers {
		err := tracker.G().FuncTrack(labelMap[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventContainerShutdown, labelMap, func() (error, model.ErrorCode) {
			err := utils.CallWithRetry(ctx, func(_ int) (bool, error) {
				innerErr := b.stopContainer(ctx, podKey, &container)

				return innerErr != nil, innerErr
			}, nil)
			if err != nil {
				return err, model.CodeContainerStopFailed
			}
			return nil, model.CodeSuccess
		})
		if err != nil {
			logger.WithError(err).WithField("containerKey", utils.GetContainerKey(podKey, container.Name)).Error("ContainerShutdownFailed")
		}
	}
}

// CreatePod is a method of VPodProvider that creates a pod
func (b *VPodProvider) CreatePod(ctx context.Context, pod *corev1.Pod) error {
	logger := log.G(ctx).WithField("podKey", utils.GetPodKey(pod))
	logger.Info("CreatePodStarted")

	// update the baseline info so the async handle logic can see them first
	podCopy := pod.DeepCopy()
	b.runtimeInfoStore.PutPod(podCopy, b.tunnel)
	b.handleContainerStart(ctx, podCopy, podCopy.Spec.Containers)

	return nil
}

// UpdatePod is a method of VPodProvider that updates a pod
func (b *VPodProvider) UpdatePod(ctx context.Context, pod *corev1.Pod) error {
	podKey := utils.GetPodKey(pod)
	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("UpdatePodStarted")

	newPod := pod.DeepCopy()

	// check pod deletion timestamp
	if newPod.ObjectMeta.DeletionTimestamp != nil {
		// skip deleted pod
		return nil
	}

	oldPod := b.runtimeInfoStore.GetPodByKey(podKey).DeepCopy()
	newContainerMap := make(map[string]corev1.Container)
	for _, container := range newPod.Spec.Containers {
		newContainerMap[container.Name] = container
	}
	shouldUpdateContainers := make([]corev1.Container, 0)
	if oldPod != nil {
		// stop first
		for _, oldContainer := range oldPod.Spec.Containers {
			// check need to stop
			if newContainer, ok := newContainerMap[oldContainer.Name]; ok && !cmp.Equal(newContainer, oldContainer) {
				// sending to stop
				shouldUpdateContainers = append(shouldUpdateContainers, oldContainer)
			} else {
				// delete from new containers
				delete(newContainerMap, oldContainer.Name)
			}
		}
		b.handleContainerShutdown(ctx, oldPod, shouldUpdateContainers)
	}

	if len(shouldUpdateContainers) == 0 {
		// no need to stop, just do nothing
		return nil
	}

	b.runtimeInfoStore.PutPod(newPod.DeepCopy(), b.tunnel)

	// only start new containers and changed containers
	startNewContainer := func() {
		b.handleContainerStart(ctx, newPod, shouldUpdateContainers)
	}

	tracker.G().Eventually(pod.Labels[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventVPodUpdate, pod.Labels, model.CodeContainerStartTimeout, func() bool {
		podFromKubernetes := &corev1.Pod{}
		err := b.client.Get(ctx, client.ObjectKey{
			Namespace: pod.Namespace,
			Name:      pod.Name,
		}, podFromKubernetes)
		if err != nil {
			logger.WithError(err).Error("Failed to get pod from k8s")
			return false
		}

		nameToContainerStatus := make(map[string]corev1.ContainerStatus)
		for _, containerStatus := range podFromKubernetes.Status.ContainerStatuses {
			nameToContainerStatus[containerStatus.Name] = containerStatus
		}

		for _, shouldUpdateContainer := range shouldUpdateContainers {
			if status, has := nameToContainerStatus[shouldUpdateContainer.Name]; has && status.State.Terminated == nil {
				return false
			}
		}
		return true
	}, time.Minute, time.Second, startNewContainer, func() {
		logger.Warn("stop old containers timeout, not start new containers")
	})

	return nil
}

// DeletePod is a method of VPodProvider that deletes a pod
func (b *VPodProvider) DeletePod(ctx context.Context, pod *corev1.Pod) error {
	podKey := utils.GetPodKey(pod)
	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("DeletePodStarted")
	if pod == nil {
		// this should never happen
		return nil
	}

	localPod := b.runtimeInfoStore.GetPodByKey(podKey)
	if localPod == nil {
		// has been deleted or not managed by current provider, just return
		return nil
	}

	// delete from curr provider
	b.runtimeInfoStore.DeletePod(podKey, b.tunnel)

	b.handleContainerShutdown(ctx, pod, pod.Spec.Containers)

	if pod.DeletionGracePeriodSeconds == nil || *pod.DeletionGracePeriodSeconds == 0 {
		// force delete, just return, skip check and delete
		logger.Warnf("Pod force delete")
		return nil
	}

	// check all containers shutdown successfully
	deletePod := func() {
		if b.client != nil {
			// delete pod with no grace period, mock kubelet
			b.client.Delete(ctx, pod, &client.DeleteOptions{
				GracePeriodSeconds: ptr.To[int64](0),
			})
		}
	}

	go tracker.G().Eventually(pod.Labels[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventVPodDelete, pod.Labels, model.CodeContainerStartTimeout, func() bool {
		nameToContainerStatus := make(map[string]corev1.ContainerStatus)
		for _, containerStatus := range pod.Status.ContainerStatuses {
			nameToContainerStatus[containerStatus.Name] = containerStatus
		}
		for _, container := range pod.Spec.Containers {
			if status, has := nameToContainerStatus[container.Name]; has && status.State.Terminated == nil {
				return false
			}
		}
		return true
	}, time.Second*25, time.Second, deletePod, deletePod)

	return nil
}

// GetPod is a method of VPodProvider that gets a pod
// This method is simply used to return the observed defaultPod by local
//
//	so the outer control loop can call CreatePod / UpdatePod / DeletePod accordingly
//	just return the defaultPod from the local store
func (b *VPodProvider) GetPod(_ context.Context, namespace, name string) (*corev1.Pod, error) {
	return b.runtimeInfoStore.GetPodByKey(namespace + "/" + name), nil
}

// GetPodStatus is a method of VPodProvider that gets the status of a pod
// This will be called repeatedly by virtual kubelet framework to get the defaultPod status
// we should query the actual runtime info and convert them in to V1PodStatus accordingly
func (b *VPodProvider) GetPodStatus(ctx context.Context, pod *corev1.Pod, bizStatus model.BizStatusData) (*corev1.PodStatus, error) {
	podStatus := &corev1.PodStatus{}
	// check pod status
	bizJarContainerCount := 0
	readyBizJarContainerCount := 0
	terminatedBizJarContainerCount := 0
	notReadyBizJarContainerCount := 0
	notInitedBizJarContainerCount := 0

	podStatus.PodIP = b.localIP
	podStatus.PodIPs = []corev1.PodIP{{IP: b.localIP}}
	podStatus.ContainerStatuses = make([]corev1.ContainerStatus, 0)

	nameToContainerStatus := make(map[string]*corev1.ContainerStatus)
	for _, cs := range pod.Status.ContainerStatuses {
		nameToContainerStatus[cs.Name] = &cs
	}

	// TODO: check all containers status only biz jar container
	for _, container := range pod.Spec.Containers {
		// only check biz jar container
		if !strings.Contains(container.Image, ".jar") {
			continue
		}
		containerStatus, err := utils.ConvertBizStatusToContainerStatus(&container, nameToContainerStatus[container.Name], &bizStatus)
		if err != nil || containerStatus == nil {
			log.G(ctx).Errorf("can't convert biz status to container status for container %s", utils.GetContainerKey(utils.GetPodKey(pod), container.Name))
			return nil, err
		} else {
			podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, *containerStatus)
		}

		bizJarContainerCount++
		if containerStatus.Ready {
			readyBizJarContainerCount++
		} else if containerStatus.State.Terminated != nil {
			terminatedBizJarContainerCount++
		} else if containerStatus.State.Waiting != nil || containerStatus.State.Running != nil {
			notReadyBizJarContainerCount++
		} else {
			// not init yet
			notInitedBizJarContainerCount++
		}
	}

	podStatus.Phase = corev1.PodPending

	if bizJarContainerCount == 0 || bizJarContainerCount == terminatedBizJarContainerCount {
		// if no biz jar container or all biz jar container terminated, pod is terminated
		podStatus.Phase = corev1.PodSucceeded
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:          "Ready",
				Status:        corev1.ConditionFalse,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
			{
				Type:          "ContainersReady",
				Status:        corev1.ConditionFalse,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
		}
		return podStatus, nil
	} else if notInitedBizJarContainerCount == bizJarContainerCount {
		podStatus.Phase = corev1.PodPending
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:          "Ready",
				Status:        corev1.ConditionFalse,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
			{
				Type:          "ContainersReady",
				Status:        corev1.ConditionFalse,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
		}
	} else if bizJarContainerCount == readyBizJarContainerCount {
		// all biz jar container is ready
		podStatus.Phase = corev1.PodRunning
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:          "Ready",
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
			{
				Type:          "ContainersReady",
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
		}
	} else if notReadyBizJarContainerCount > 0 || (readyBizJarContainerCount > 0 && readyBizJarContainerCount < bizJarContainerCount) {
		podStatus.Phase = corev1.PodRunning
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:          "Ready",
				Status:        corev1.ConditionFalse,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
			{
				Type:          "ContainersReady",
				Status:        corev1.ConditionFalse,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
		}
	} else {
		podStatus.Phase = corev1.PodPending
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:          "Ready",
				Status:        corev1.ConditionFalse,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
			{
				Type:          "ContainersReady",
				Status:        corev1.ConditionFalse,
				LastProbeTime: metav1.NewTime(time.Now()),
			},
		}
	}

	return podStatus, nil
}

func (b *VPodProvider) GetPods(_ context.Context) ([]*corev1.Pod, error) {
	return b.runtimeInfoStore.GetPods(), nil
}
