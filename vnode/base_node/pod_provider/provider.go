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
	"errors"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"sort"
	"sync"
	"time"

	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/queue"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
)

var _ nodeutil.Provider = &BaseProvider{}
var _ virtual_kubelet.PodNotifier = &BaseProvider{}

type BaseProvider struct {
	Namespace        string
	nodeID           string
	localIP          string
	k8sClient        kubernetes.Interface
	runtimeInfoStore *RuntimeInfoStore

	tunnel tunnel.Tunnel

	installOperationQueue   *queue.Queue
	uninstallOperationQueue *queue.Queue

	bizInfosCache bizInfosCache
	port          int

	notify func(pod *corev1.Pod)
}

func (b *BaseProvider) NotifyPods(_ context.Context, cb func(*corev1.Pod)) {
	b.notify = cb
}

type bizInfosCache struct {
	sync.Mutex

	LatestBizInfos []ark.ArkBizInfo
}

func NewBaseProvider(namespace, localIP, nodeID string, k8sClient kubernetes.Interface, t tunnel.Tunnel) *BaseProvider {
	provider := &BaseProvider{
		Namespace:        namespace,
		localIP:          localIP,
		nodeID:           nodeID,
		k8sClient:        k8sClient,
		tunnel:           t,
		runtimeInfoStore: NewRuntimeInfoStore(),
		notify: func(pod *corev1.Pod) {
			logrus.Info("default pod notifier notified", pod.Name)
		},
	}

	provider.installOperationQueue = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"bizInstallOperationQueue",
		provider.handleInstallOperation,
		func(ctx context.Context, key string, timesTried int, originallyAdded time.Time, err error) (*time.Duration, error) {
			duration := time.Millisecond * 100
			return &duration, nil
		},
	)

	provider.uninstallOperationQueue = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"bizUninstallOperationQueue",
		provider.handleUnInstallOperation,
		func(ctx context.Context, key string, timesTried int, originallyAdded time.Time, err error) (*time.Duration, error) {
			duration := time.Millisecond * 100
			return &duration, nil
		},
	)

	return provider
}

func (b *BaseProvider) Run(ctx context.Context) {
	go b.installOperationQueue.Run(ctx, 1)
	go b.uninstallOperationQueue.Run(ctx, 1)
	go utils.TimedTaskWithInterval(ctx, time.Second*5, b.checkAndUninstallDanglingBiz)
	go utils.TimedTaskWithInterval(ctx, time.Minute, b.syncAllPodStatus)
}

// checkAndUninstallDanglingBiz mainly process a pod being deleted before biz activated, in resolved status, biz can't uninstall
func (b *BaseProvider) checkAndUninstallDanglingBiz(ctx context.Context) {
	logger := log.G(ctx)
	bindingModels := make(map[string]bool)
	for _, pod := range b.runtimeInfoStore.GetPods() {
		if pod.DeletionTimestamp != nil {
			// skip pod in deletion
			continue
		}
		podKey := utils.ModelUtil.GetPodKey(pod)
		bizModels := b.runtimeInfoStore.GetRelatedBizModels(podKey)
		for _, bizModel := range bizModels {
			bizIdentity := utils.ModelUtil.GetBizIdentityFromBizModel(bizModel)
			bindingModels[bizIdentity] = true
		}
	}
	// query all modules loading now, if not in binding, queue to uninstall
	bizInfos := b.queryAllBiz(ctx)
	for _, bizInfo := range bizInfos {
		if bizInfo.BizState != "ACTIVATED" && bizInfo.BizState != "DEACTIVATED" {
			continue
		}
		bizIdentity := utils.ModelUtil.GetBizIdentityFromBizInfo(&bizInfo)
		if !bindingModels[bizIdentity] {
			// not binding,send to uninstall
			b.uninstallOperationQueue.Enqueue(ctx, bizIdentity)
			logger.WithField("bizName", bizInfo.BizName).WithField("bizVersion", bizInfo.BizVersion).Info("ItemEnqueued")
		}
	}
}

func (b *BaseProvider) syncAllPodStatus(ctx context.Context) {
	logger := log.G(ctx)
	pods := b.runtimeInfoStore.GetPods()
	// sort by create time
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].CreationTimestamp.UnixMilli() > pods[j].CreationTimestamp.UnixMilli()
	})
	for _, pod := range pods {
		if err := b.updatePodStatus(ctx, pod); err != nil {
			logger.WithError(err).Error("update pod status error")
		}
	}
}

func (b *BaseProvider) updatePodStatus(ctx context.Context, pod *corev1.Pod) error {
	podStatus, err := b.GetPodStatus(ctx, pod.Namespace, pod.Name)
	if err != nil {
		return err
	}

	podInfo := pod.DeepCopy()
	podStatus.DeepCopyInto(&podInfo.Status)
	b.notify(podInfo)
	return nil
}

func (b *BaseProvider) SyncBizInfo(bizInfos []ark.ArkBizInfo) {
	b.bizInfosCache.Lock()
	b.bizInfosCache.LatestBizInfos = bizInfos
	b.bizInfosCache.Unlock()

	b.syncAllPodStatus(context.Background())
}

func (b *BaseProvider) queryAllBiz(_ context.Context) []ark.ArkBizInfo {
	b.bizInfosCache.Lock()
	defer b.bizInfosCache.Unlock()
	return b.bizInfosCache.LatestBizInfos
}

func (b *BaseProvider) queryBiz(ctx context.Context, bizIdentity string) *ark.ArkBizInfo {
	infos := b.queryAllBiz(ctx)

	for _, info := range infos {
		infoIdentity := utils.ModelUtil.GetBizIdentityFromBizInfo(&info)
		if infoIdentity == bizIdentity {
			return &info
		}
	}

	return nil
}

func (b *BaseProvider) installBiz(ctx context.Context, bizModel *ark.BizModel) error {
	return b.tunnel.InstallBiz(ctx, b.nodeID, bizModel)
}

func (b *BaseProvider) unInstallBiz(ctx context.Context, bizModel *ark.BizModel) error {
	return b.tunnel.UninstallBiz(ctx, b.nodeID, bizModel)
}

func (b *BaseProvider) handleInstallOperation(ctx context.Context, bizIdentity string) error {
	logger := log.G(ctx).WithField("bizIdentity", bizIdentity)
	logger.Info("HandleBizInstallOperationStarted")

	bizModel := b.runtimeInfoStore.GetBizModel(bizIdentity)
	if bizModel == nil {
		// for installation, this should never happen, no retry here
		logger.Error("Installing non-existent pod")
		return nil
	}
	bizInfo := b.queryBiz(ctx, bizIdentity)

	if bizInfo != nil && bizInfo.BizState == "ACTIVATED" {
		logger.Info("BizAlreadyActivated")
		return nil
	}

	if bizInfo != nil && bizInfo.BizState == "RESOLVED" {
		// process concurrent install operation
		logger.Info("BizInstalling")
		return nil
	}

	if bizInfo != nil && bizInfo.BizState == "DEACTIVATED" {
		logger.Error("BizInstalledButNotActivated")
		return errors.New("BizInstalledButNotActivated")
	}

	if err := b.installBiz(ctx, bizModel); err != nil {
		logger.WithError(err).Error("InstallBizFailed")
		return err
	}

	logger.Info("HandleBizInstallOperationFinished")
	return nil
}

func (b *BaseProvider) handleUnInstallOperation(ctx context.Context, bizIdentity string) error {
	logger := log.G(ctx).WithField("bizIdentity", bizIdentity)
	logger.Info("HandleUnInstallOperationStarted")

	bizInfo := b.queryBiz(ctx, bizIdentity)

	if bizInfo != nil {
		// local installed, call uninstall
		if err := b.unInstallBiz(ctx, &ark.BizModel{
			BizName:    bizInfo.BizName,
			BizVersion: bizInfo.BizVersion,
		}); err != nil {
			logger.WithError(err).Error("UnInstallBizFailed")
			return err
		}
	}

	logger.Info("HandleBizUninstallOperationFinished")
	return nil
}

// CreatePod directly install a biz bundle to base
func (b *BaseProvider) CreatePod(ctx context.Context, pod *corev1.Pod) error {
	logger := log.G(ctx).WithField("podKey", utils.ModelUtil.GetPodKey(pod))
	logger.Info("CreatePodStarted")

	// update the baseline info so the async handle logic can see them first
	b.runtimeInfoStore.PutPod(pod.DeepCopy())
	bizModels := utils.ModelUtil.GetBizModelsFromCoreV1Pod(pod)
	for _, bizModel := range bizModels {
		b.installOperationQueue.Enqueue(ctx, utils.ModelUtil.GetBizIdentityFromBizModel(bizModel))
		logger.WithField("bizName", bizModel.BizName).WithField("bizVersion", bizModel.BizVersion).Info("ItemEnqueued")
	}

	return nil
}

// UpdatePod install directly
func (b *BaseProvider) UpdatePod(ctx context.Context, pod *corev1.Pod) error {
	podKey := utils.ModelUtil.GetPodKey(pod)
	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("UpdatePodStarted")

	newModels := utils.ModelUtil.GetBizModelsFromCoreV1Pod(pod)

	// check pod deletion timestamp
	if pod.ObjectMeta.DeletionTimestamp == nil {
		b.runtimeInfoStore.PutPod(pod.DeepCopy())
		// not in deletion, install new models
		for _, newModel := range newModels {
			b.installOperationQueue.Enqueue(ctx, utils.ModelUtil.GetBizIdentityFromBizModel(newModel))
			logger.WithField("bizName", newModel.BizName).WithField("bizVersion", newModel.BizVersion).Info("ItemEnqueued")
		}
	}

	return nil
}

// DeletePod directly uninstall biz  from base
func (b *BaseProvider) DeletePod(ctx context.Context, pod *corev1.Pod) error {
	podKey := utils.ModelUtil.GetPodKey(pod)
	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("DeletePodStarted")

	// check is deleted
	b.runtimeInfoStore.DeletePod(podKey)

	if b.k8sClient != nil {
		// delete pod with no grace period, mock kubelet
		return b.k8sClient.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{
			// grace period for base pod controller deleting target finalizer
			GracePeriodSeconds: ptr.To[int64](0),
		})
	}
	return nil
}

// GetPod this method is simply used to return the observed defaultPod by local
//
//	so the outer control loop can call CreatePod / UpdatePod / DeletePod accordingly
//	just return the defaultPod from the local store
func (b *BaseProvider) GetPod(_ context.Context, namespace, name string) (*corev1.Pod, error) {
	return b.runtimeInfoStore.GetPodByKey(namespace + "/" + name), nil
}

// GetPodStatus this will be called repeatedly by virtual kubelet framework to get the defaultPod status
// we should query the actual runtime info and translate them in to V1PodStatus accordingly
func (b *BaseProvider) GetPodStatus(ctx context.Context, namespace, name string) (*corev1.PodStatus, error) {
	podKey := namespace + "/" + name
	pod := b.runtimeInfoStore.GetPodByKey(podKey)
	podStatus := &corev1.PodStatus{}
	if pod == nil {
		podStatus.Phase = corev1.PodSucceeded
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:   "module.koupleless.io/installed",
				Status: corev1.ConditionFalse,
			},
			{
				Type:   "module.koupleless.io/ready",
				Status: corev1.ConditionFalse,
			},
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
	// not in deletion
	bizModels := utils.ModelUtil.GetBizModelsFromCoreV1Pod(pod)

	bizInfos := b.queryAllBiz(ctx)
	// bizIdentity
	bizRuntimeInfos := make(map[string]*ark.ArkBizInfo)
	for _, info := range bizInfos {
		bizRuntimeInfos[utils.ModelUtil.GetBizIdentityFromBizInfo(&info)] = &info
	}
	// bizName -> container status
	containerStatuses := make(map[string]*corev1.ContainerStatus)
	/**
	if further info is provided, we can set the time accordingly
	failedTime would be the earliest time of the failed container
	successTime would be the latest time of the success container
	startTime would be the earliest time of the all container
	*/
	for _, bizModel := range bizModels {
		info := bizRuntimeInfos[utils.ModelUtil.GetBizIdentityFromBizModel(bizModel)]
		containerStatus := utils.ModelUtil.TranslateArkBizInfoToV1ContainerStatus(bizModel, info)
		containerStatuses[bizModel.BizName] = containerStatus

		if !containerStatus.Ready {
			isAllContainerReady = false
		}

		if containerStatus.State.Terminated != nil {
			isSomeContainerFailed = true
		}
	}

	podStatus.PodIP = b.localIP
	podStatus.PodIPs = []corev1.PodIP{{IP: b.localIP}}

	podStatus.ContainerStatuses = make([]corev1.ContainerStatus, 0)
	for _, status := range containerStatuses {
		podStatus.ContainerStatuses = append(podStatus.ContainerStatuses, *status)
	}

	podStatus.Phase = corev1.PodPending
	if isAllContainerReady {
		podStatus.Phase = corev1.PodRunning
		podStatus.Conditions = []corev1.PodCondition{
			{
				Type:   "module.koupleless.io/installed",
				Status: corev1.ConditionTrue,
			},
			{
				Type:   "module.koupleless.io/ready",
				Status: corev1.ConditionTrue,
			},
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
				Type:   "basement.koupleless.io/installed",
				Status: corev1.ConditionFalse,
			},
			{
				Type:   "basement.koupleless.io/ready",
				Status: corev1.ConditionFalse,
			},
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

func (b *BaseProvider) GetPods(_ context.Context) ([]*corev1.Pod, error) {
	return b.runtimeInfoStore.GetPods(), nil
}
