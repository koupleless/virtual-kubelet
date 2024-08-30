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
	"github.com/koupleless/virtual-kubelet/common/tracker"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
	"time"

	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/queue"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
)

var _ nodeutil.Provider = &VPodProvider{}
var _ virtual_kubelet.PodNotifier = &VPodProvider{}

type VPodProvider struct {
	Namespace        string
	nodeID           string
	localIP          string
	client           client.Client
	runtimeInfoStore *RuntimeInfoStore

	tunnel tunnel.Tunnel

	startOperationQueue    *queue.Queue
	shutdownOperationQueue *queue.Queue

	port int

	notify func(pod *corev1.Pod)
}

func (b *VPodProvider) NotifyPods(_ context.Context, cb func(*corev1.Pod)) {
	b.notify = cb
}

func NewVPodProvider(namespace, localIP, nodeID string, client client.Client, t tunnel.Tunnel) *VPodProvider {
	provider := &VPodProvider{
		Namespace:        namespace,
		localIP:          localIP,
		nodeID:           nodeID,
		client:           client,
		tunnel:           t,
		runtimeInfoStore: NewRuntimeInfoStore(),
	}

	provider.startOperationQueue = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"containerStartOperationQueue",
		provider.handleStartOperation,
		func(ctx context.Context, key string, timesTried int, originallyAdded time.Time, err error) (*time.Duration, error) {
			duration := time.Millisecond * 100
			return &duration, nil
		},
	)

	provider.shutdownOperationQueue = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"containerShutdownOperationQueue",
		provider.handleShutdownOperation,
		func(ctx context.Context, key string, timesTried int, originallyAdded time.Time, err error) (*time.Duration, error) {
			duration := time.Millisecond * 100
			return &duration, nil
		},
	)

	return provider
}

func (b *VPodProvider) Run(ctx context.Context) {
	go b.startOperationQueue.Run(ctx, 1)
	go b.shutdownOperationQueue.Run(ctx, 1)
	go utils.TimedTaskWithInterval(ctx, time.Minute, b.syncAllPodStatus)
}

func (b *VPodProvider) syncAllPodStatus(ctx context.Context) {
	logger := log.G(ctx)
	pods := b.runtimeInfoStore.GetPods()
	// sort by create time
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].CreationTimestamp.UnixMilli() > pods[j].CreationTimestamp.UnixMilli()
	})
	for _, pod := range pods {
		if err := b.updatePodStatusToKubernetes(ctx, pod); err != nil {
			logger.WithError(err).Error("update pod status error")
		}
	}
}

func (b *VPodProvider) syncRelatedPodStatus(ctx context.Context, podKey, containerName string) {
	logger := log.G(ctx)
	if podKey != model.PodKeyAll {
		pod := b.runtimeInfoStore.GetPodByKey(podKey)
		if err := b.updatePodStatusToKubernetes(ctx, pod); err != nil {
			logger.WithError(err).Error("update pod status error")
		}
	} else {
		podKeys := b.runtimeInfoStore.GetRelatedPodKeysByContainerName(containerName)
		pods := make([]*corev1.Pod, 0)
		for _, key := range podKeys {
			pod := b.runtimeInfoStore.GetPodByKey(key)
			pods = append(pods, pod)
		}
		// sort by create time
		sort.Slice(pods, func(i, j int) bool {
			return pods[i].CreationTimestamp.UnixMilli() > pods[j].CreationTimestamp.UnixMilli()
		})
		for _, pod := range pods {
			if err := b.updatePodStatusToKubernetes(ctx, pod); err != nil {
				logger.WithError(err).Error("update pod status error")
			}
		}
	}

}

func (b *VPodProvider) updatePodStatusToKubernetes(ctx context.Context, pod *corev1.Pod) error {
	podStatus, err := b.GetPodStatus(ctx, pod.Namespace, pod.Name)
	if err != nil {
		return err
	}

	podInfo := pod.DeepCopy()
	podStatus.DeepCopyInto(&podInfo.Status)
	b.notify(podInfo)
	return nil
}

func (b *VPodProvider) SyncContainerInfo(ctx context.Context, containerInfos []model.ContainerStatusData) {
	b.runtimeInfoStore.SyncContainerInfo(containerInfos)
	b.syncAllPodStatus(ctx)
}

func (b *VPodProvider) SyncSingleContainerInfo(ctx context.Context, info model.ContainerStatusData) {
	b.runtimeInfoStore.PutContainerInfo(info)
	b.syncRelatedPodStatus(ctx, info.PodKey, info.Name)
}

func (b *VPodProvider) queryAllContainerStatus(_ context.Context) []*model.ContainerStatusData {
	return b.runtimeInfoStore.GetLatestContainerInfos()
}

func (b *VPodProvider) queryContainerStatus(_ context.Context, podKey string, container *corev1.Container) *model.ContainerStatusData {
	containerUniqueKey := b.tunnel.GetContainerUniqueKey(podKey, container)
	return b.runtimeInfoStore.GetLatestContainerInfoByContainerKey(containerUniqueKey)
}

func (b *VPodProvider) startContainer(ctx context.Context, podKey string, container *corev1.Container) error {
	return b.tunnel.StartContainer(ctx, b.nodeID, podKey, container)
}

func (b *VPodProvider) stopContainer(ctx context.Context, podKey string, container *corev1.Container) error {
	return b.tunnel.ShutdownContainer(ctx, b.nodeID, podKey, container)
}

func (b *VPodProvider) handleStartOperation(ctx context.Context, containerKey string) error {
	logger := log.G(ctx).WithField("containerKey", containerKey)
	logger.Info("HandleContainerStartOperation")

	podKey := utils.GetPodKeyFromContainerKey(containerKey)
	podLocal := b.runtimeInfoStore.GetPodByKey(podKey)
	var labelMap map[string]string
	if podLocal != nil {
		labelMap = podLocal.Labels
	}
	if labelMap == nil {
		labelMap = make(map[string]string)
	}

	return tracker.G().FuncTrack(labelMap[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventContainerStart, labelMap, func() (error, model.ErrorCode) {
		container := b.runtimeInfoStore.GetContainer(containerKey)
		if container == nil {
			// this should never happen, no retry here
			return nil, model.CodeSuccess
		}
		containerStatus := b.queryContainerStatus(ctx, podKey, container)

		if containerStatus != nil && containerStatus.State == model.ContainerStateActivated {
			logger.Info("ContainerAlreadyActivated")
			return nil, model.CodeSuccess
		}

		if containerStatus != nil && containerStatus.State == model.ContainerStateResolved {
			// process concurrent install operation
			logger.Info("ContainerStarting")
			return nil, model.CodeSuccess
		}

		if containerStatus != nil && containerStatus.State == model.ContainerStateDeactivated {
			logger.Info("ContainerDeactivated, restart")
		}

		if err := b.startContainer(ctx, podKey, container); err != nil {
			return err, model.CodeContainerStartFailed
		}
		return nil, model.CodeSuccess
	})
}

func (b *VPodProvider) handleShutdownOperation(ctx context.Context, containerKey string) error {
	logger := log.G(ctx).WithField("containerKey", containerKey)
	logger.Info("HandleStopContainerOperationStarted")

	podKey := utils.GetPodKeyFromContainerKey(containerKey)
	podLocal := b.runtimeInfoStore.GetPodByKey(podKey)
	var labelMap map[string]string
	if podLocal != nil {
		labelMap = podLocal.Labels
	}
	if labelMap == nil {
		labelMap = make(map[string]string)
	}

	return tracker.G().FuncTrack(labelMap[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventContainerShutdown, labelMap, func() (error, model.ErrorCode) {
		container := b.runtimeInfoStore.GetContainer(containerKey)

		if container != nil {
			defer b.runtimeInfoStore.DeleteContainer(containerKey)
			if err := b.stopContainer(ctx, podKey, container); err != nil {
				logger.WithError(err).Error("StopContainerFailed")
				return err, model.CodeContainerStopFailed
			}
		}
		return nil, model.CodeSuccess
	})
}

func (b *VPodProvider) CreatePod(ctx context.Context, pod *corev1.Pod) error {
	logger := log.G(ctx).WithField("podKey", utils.GetPodKey(pod))
	logger.Info("CreatePodStarted")

	// update the baseline info so the async handle logic can see them first
	podCopy := pod.DeepCopy()
	b.runtimeInfoStore.PutPod(podCopy)
	for _, container := range podCopy.Spec.Containers {
		podKey := utils.GetPodKey(podCopy)
		containerKey := utils.GetContainerKey(podKey, container.Name)
		b.startOperationQueue.Enqueue(ctx, containerKey)
		logger.WithField("podKey", podKey).WithField("containerName", container.Name).Info("ItemEnqueued")
	}

	return nil
}

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
	if oldPod != nil {
		oldPopKey := utils.GetPodKey(oldPod)
		// stop first
		for _, container := range oldPod.Spec.Containers {
			// sending to stop
			containerKey := utils.GetContainerKey(oldPopKey, container.Name)
			b.shutdownOperationQueue.Enqueue(ctx, containerKey)
			logger.WithField("podKey", podKey).WithField("containerName", container.Name).Info("ItemEnqueued")
		}
	}

	b.runtimeInfoStore.PutPod(pod.DeepCopy())

	// start a go runtime, check old containers shutdown successfully, then start new containers
	startNewContainer := func() {
		for _, container := range newPod.Spec.Containers {
			containerKey := utils.GetContainerKey(podKey, container.Name)
			b.startOperationQueue.Enqueue(ctx, containerKey)
			logger.WithField("podKey", podKey).WithField("containerName", container.Name).Info("ItemEnqueued")
		}
	}
	go tracker.G().Eventually(pod.Labels[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventVPodUpdate, pod.Labels, model.CodeContainerStartTimeout, func() bool {
		for _, oldContainer := range oldPod.Spec.Containers {
			oldPodKey := utils.GetPodKey(oldPod)
			if b.queryContainerStatus(ctx, oldPodKey, &oldContainer) != nil {
				return false
			}
		}
		return true
	}, time.Minute, time.Second, startNewContainer, startNewContainer)

	return nil
}

func (b *VPodProvider) DeletePod(ctx context.Context, pod *corev1.Pod) error {
	podKey := utils.GetPodKey(pod)
	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("DeletePodStarted")

	localPod := b.runtimeInfoStore.GetPodByKey(podKey).DeepCopy()
	if localPod == nil {
		localPod = pod.DeepCopy()
	}
	for _, container := range localPod.Spec.Containers {
		// sending to delete
		containerKey := utils.GetContainerKey(podKey, container.Name)
		b.shutdownOperationQueue.Enqueue(ctx, containerKey)
		logger.WithField("podKey", podKey).WithField("containerName", container.Name).Info("ItemEnqueued")
	}

	if pod.DeletionGracePeriodSeconds == nil || *pod.DeletionGracePeriodSeconds == 0 {
		// force delete, just return
		return nil
	}

	// check all containers shutdown successfully
	deletePod := func() {
		b.runtimeInfoStore.DeletePod(podKey)

		if b.client != nil {
			// delete pod with no grace period, mock kubelet
			err := b.client.Delete(ctx, pod, &client.DeleteOptions{
				GracePeriodSeconds: ptr.To[int64](0),
			})
			if err != nil {
				// might have been deleted manually or exceeded grace period, logger error msg
				logger.WithError(err).Error("Pod has been deleted in k8s")
			}
		}
	}

	go tracker.G().Eventually(pod.Labels[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventVPodDelete, pod.Labels, model.CodeContainerStartTimeout, func() bool {
		for _, oldContainer := range localPod.Spec.Containers {
			oldPodKey := utils.GetPodKey(localPod)
			if b.queryContainerStatus(ctx, oldPodKey, &oldContainer) != nil {
				return false
			}
		}
		return true
	}, time.Second*25, time.Second, deletePod, deletePod)

	return nil
}

// GetPod this method is simply used to return the observed defaultPod by local
//
//	so the outer control loop can call CreatePod / UpdatePod / DeletePod accordingly
//	just return the defaultPod from the local store
func (b *VPodProvider) GetPod(_ context.Context, namespace, name string) (*corev1.Pod, error) {
	return b.runtimeInfoStore.GetPodByKey(namespace + "/" + name), nil
}

// GetPodStatus this will be called repeatedly by virtual kubelet framework to get the defaultPod status
// we should query the actual runtime info and translate them in to V1PodStatus accordingly
func (b *VPodProvider) GetPodStatus(ctx context.Context, namespace, name string) (*corev1.PodStatus, error) {
	podKey := namespace + "/" + name
	pod := b.runtimeInfoStore.GetPodByKey(podKey)
	podStatus := &corev1.PodStatus{}
	if pod == nil {
		podStatus.Phase = corev1.PodSucceeded
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:   "Ready",
				Status: corev1.ConditionFalse,
			},
			{
				Type:   "ContainersReady",
				Status: corev1.ConditionFalse,
			},
		}
		return podStatus, nil
	}
	// check pod in deletion
	isAllContainerReady := true
	isSomeContainerFailed := false

	podStatus.PodIP = b.localIP
	podStatus.PodIPs = []corev1.PodIP{{IP: b.localIP}}

	podStatus.ContainerStatuses = make([]corev1.ContainerStatus, 0)
	for _, container := range pod.Spec.Containers {
		containerStatusFromTunnel := b.queryContainerStatus(ctx, podKey, &container)
		containerStatus := utils.TranslateContainerStatusFromTunnelToContainerStatus(container, containerStatusFromTunnel)

		if !containerStatus.Ready {
			isAllContainerReady = false
		}

		if containerStatus.State.Terminated != nil {
			isSomeContainerFailed = true
		}

		podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, containerStatus)
	}

	podStatus.Phase = corev1.PodPending
	if isAllContainerReady {
		podStatus.Phase = corev1.PodRunning
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:   "Ready",
				Status: corev1.ConditionTrue,
			},
			{
				Type:   "ContainersReady",
				Status: corev1.ConditionTrue,
			},
		}
	}

	if isSomeContainerFailed {
		podStatus.Phase = corev1.PodRunning
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:   "Ready",
				Status: corev1.ConditionFalse,
			},
			{
				Type:   "ContainersReady",
				Status: corev1.ConditionFalse,
			},
		}
	}

	return podStatus, nil
}

func (b *VPodProvider) GetPods(_ context.Context) ([]*corev1.Pod, error) {
	return b.runtimeInfoStore.GetPods(), nil
}
