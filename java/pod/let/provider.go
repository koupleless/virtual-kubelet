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

package let

import (
	"context"
	"errors"
	"github.com/koupleless/virtual-kubelet/java/model"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"sync"
	"time"

	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/queue"
	"github.com/koupleless/virtual-kubelet/java/common"
	"github.com/prometheus/client_model/go"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
)

var _ nodeutil.Provider = &BaseProvider{}

type BaseProvider struct {
	namespace string

	localIP                 string
	arkService              ark.Service
	modelUtils              common.ModelUtils
	runtimeInfoStore        *RuntimeInfoStore
	installOperationQueue   *queue.Queue
	uninstallOperationQueue *queue.Queue

	bas *baseArkletStatus

	port int
}

type baseArkletStatus struct {
	sync.Mutex
	lastStatus bool
}

func NewBaseProvider(namespace string, arkService ark.Service) *BaseProvider {
	localIP := os.Getenv("BASE_POD_IP")
	if localIP == "" {
		localIP = model.LOOP_BACK_IP
	}
	provider := &BaseProvider{
		namespace:        namespace,
		localIP:          localIP,
		arkService:       arkService,
		modelUtils:       common.ModelUtils{},
		runtimeInfoStore: NewRuntimeInfoStore(),
		bas: &baseArkletStatus{
			lastStatus: true,
		},
		port: model.ARK_SERVICE_PORT,
	}

	provider.installOperationQueue = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"bizInstallOperationQueue",
		provider.handleInstallOperation,
		// todo: more complicated retry logic
		func(ctx context.Context, key string, timesTried int, originallyAdded time.Time, err error) (*time.Duration, error) {
			duration := time.Millisecond * 100
			return &duration, nil
		},
	)

	provider.uninstallOperationQueue = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"bizUninstallOperationQueue",
		provider.handleUnInstallOperation,
		// todo: more complicated retry logic
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
	go b.checkAndUninstallDanglingBiz(context.WithValue(ctx, "timed task", "check and uninstall dangling biz"), time.Second*5)
}

// checkAndUninstallDanglingBiz mainly process a pod being deleted before biz activated, in resolved status, biz can't uninstall
func (b *BaseProvider) checkAndUninstallDanglingBiz(ctx context.Context, interval time.Duration) {
	logger := log.G(ctx)
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			bindingModels := make(map[string]bool)
			for _, pod := range b.runtimeInfoStore.GetPods() {
				if pod.DeletionTimestamp != nil {
					// skip pod in deletion
					continue
				}
				podKey := b.modelUtils.GetPodKey(pod)
				bizModels := b.runtimeInfoStore.GetRelatedBizModels(podKey)
				for _, bizModel := range bizModels {
					bizIdentity := b.modelUtils.GetBizIdentityFromBizModel(bizModel)
					bindingModels[bizIdentity] = true
				}
			}
			// query all modules loading now, if not in binding, queue to uninstall
			bizInfos, err := b.queryAllBiz(ctx)
			if err != nil {
				logger.WithError(err).Error("query biz info error")
				continue
			}
			for _, bizInfo := range bizInfos {
				if bizInfo.BizState == "RESOLVED" {
					continue
				}
				bizIdentity := b.modelUtils.GetBizIdentityFromBizInfo(&bizInfo)
				if !bindingModels[bizIdentity] {
					// not binding,send to uninstall
					b.uninstallOperationQueue.Enqueue(ctx, bizIdentity)
					logger.WithField("bizName", bizInfo.BizName).WithField("bizVersion", bizInfo.BizVersion).Info("ItemEnqueued")
				}
			}
		}
	}
}

func (b *BaseProvider) queryAllBiz(ctx context.Context) ([]ark.ArkBizInfo, error) {
	resp, err := b.arkService.QueryAllBiz(ctx, ark.QueryAllArkBizRequest{
		HostName: model.LOOP_BACK_IP,
		Port:     b.port,
	})
	if err != nil {
		log.G(ctx).WithError(err).Error("QueryAllBizFailed")
		return nil, err
	}

	if resp.Code != "SUCCESS" {
		err = errors.New(resp.Message)
		log.G(ctx).WithError(err).Error("QueryAllBizFailed")
		return nil, err
	}

	return resp.Data, nil
}

func (b *BaseProvider) queryBiz(ctx context.Context, bizIdentity string) (*ark.ArkBizInfo, error) {
	infos, err := b.queryAllBiz(ctx)
	if err != nil {
		return nil, err
	}

	for _, info := range infos {
		infoIdentity := b.modelUtils.GetBizIdentityFromBizInfo(&info)
		if infoIdentity == bizIdentity {
			return &info, nil
		}
	}

	return nil, nil
}

func (b *BaseProvider) installBiz(ctx context.Context, bizModel *ark.BizModel) error {
	if err := b.arkService.InstallBiz(ctx, ark.InstallBizRequest{
		BizModel: *bizModel,
		TargetContainer: ark.ArkContainerRuntimeInfo{
			RunType:    ark.ArkContainerRunTypeLocal,
			Coordinate: model.LOOP_BACK_IP,
			Port:       &b.port,
		},
	}); err != nil {
		log.G(ctx).WithError(err).Info("InstallBizFailed")
		return err
	}
	return nil
}

func (b *BaseProvider) unInstallBiz(ctx context.Context, bizModel *ark.BizModel) error {
	if err := b.arkService.UnInstallBiz(ctx, ark.UnInstallBizRequest{
		BizModel: *bizModel,
		TargetContainer: ark.ArkContainerRuntimeInfo{
			RunType:    ark.ArkContainerRunTypeLocal,
			Coordinate: model.LOOP_BACK_IP,
			Port:       &b.port,
		},
	}); err != nil {
		log.G(ctx).WithError(err).Info("UnInstallBizFailed")
		return err
	}
	return nil
}

func (b *BaseProvider) handleInstallOperation(ctx context.Context, bizIdentity string) error {
	logger := log.G(ctx).WithField("bizIdentity", bizIdentity)
	logger.Info("HandleBizInstallOperationStarted")

	bizModel := b.runtimeInfoStore.GetBizModel(bizIdentity)
	if bizModel == nil {
		// for installation, this should never happen, no retry here
		logger.Error("Installing non-existent defaultPod")
		return nil
	}
	bizInfo, err := b.queryBiz(ctx, bizIdentity)
	if err != nil {
		logger.WithError(err).Error("QueryBizFailed")
		return err
	}

	if bizInfo != nil && bizInfo.BizState == "ACTIVATED" {
		logger.Info("BizAlreadyActivated")
		return nil
	}

	if bizInfo != nil && bizInfo.BizState == "RESOLVED" {
		// process concurrent install operation
		logger.Info("BizInstalling")
		return nil
	}

	if bizInfo != nil && bizInfo.BizState != "DEACTIVATED" {
		// todo: support retry accordingly
		//       we should check the related defaultPod failed strategy and retry accordingly
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

	bizInfo, err := b.queryBiz(ctx, bizIdentity)
	if err != nil {
		logger.WithError(err).Error("QueryBizFailed")
		return err
	}

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
	logger := log.G(ctx).WithField("podKey", b.modelUtils.GetPodKey(pod))
	logger.Info("CreatePodStarted")

	// update the baseline info so the async handle logic can see them first
	b.runtimeInfoStore.PutPod(pod.DeepCopy())
	bizModels := b.modelUtils.GetBizModelsFromCoreV1Pod(pod)
	for _, bizModel := range bizModels {
		b.installOperationQueue.Enqueue(ctx, b.modelUtils.GetBizIdentityFromBizModel(bizModel))
		logger.WithField("bizName", bizModel.BizName).WithField("bizVersion", bizModel.BizVersion).Info("ItemEnqueued")
	}

	return nil
}

// UpdatePod install directly
func (b *BaseProvider) UpdatePod(ctx context.Context, pod *corev1.Pod) error {
	podKey := b.modelUtils.GetPodKey(pod)
	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("UpdatePodStarted")

	newModels := b.modelUtils.GetBizModelsFromCoreV1Pod(pod)

	// check pod deletion timestamp
	if pod.ObjectMeta.DeletionTimestamp == nil {
		b.runtimeInfoStore.PutPod(pod.DeepCopy())
		// not in deletion, install new models
		for _, newModel := range newModels {
			b.installOperationQueue.Enqueue(ctx, b.modelUtils.GetBizIdentityFromBizModel(newModel))
			logger.WithField("bizName", newModel.BizName).WithField("bizVersion", newModel.BizVersion).Info("ItemEnqueued")
		}
	}

	return nil
}

// DeletePod directly uninstall biz  from base
func (b *BaseProvider) DeletePod(ctx context.Context, pod *corev1.Pod) error {
	podKey := b.modelUtils.GetPodKey(pod)
	logger := log.G(ctx).WithField("podKey", podKey)
	logger.Info("DeletePodStarted")

	// check is deleted
	b.runtimeInfoStore.DeletePod(podKey)

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
// todo: 目前 defaultPod 的 status 状态是直接从 arklet 中查询和转换的，但是没有时间戳信息，所以无法提交对应的事件和开始时间。
//
//	当未来 arklet 提供了更多的信息时，我们可以相应的设置时间。
func (b *BaseProvider) GetPodStatus(ctx context.Context, namespace, name string) (*corev1.PodStatus, error) {
	podKey := namespace + "/" + name
	pod := b.runtimeInfoStore.GetPodByKey(podKey)
	podStatus := &corev1.PodStatus{}
	logger := log.G(ctx)
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
	bizModels := b.modelUtils.GetBizModelsFromCoreV1Pod(pod)

	bizInfos, err := b.queryAllBiz(ctx)
	b.bas.Lock()
	if err != nil {
		b.bas.lastStatus = false
	} else {
		// check last status is down
		if b.bas.lastStatus == false {
			// do module playback
			logger.Info("StartModulePlayback")
			for _, podBizModels := range b.runtimeInfoStore.podKeyToBizModels {
				for _, bizModel := range podBizModels {
					b.installOperationQueue.Enqueue(ctx, b.modelUtils.GetBizIdentityFromBizModel(bizModel))
					logger.WithField("bizName", bizModel.BizName).WithField("bizVersion", bizModel.BizVersion).Info("ItemEnqueued")
				}
			}
		}
		b.bas.lastStatus = true
	}
	b.bas.Unlock()

	// bizIdentity
	bizRuntimeInfos := make(map[string]*ark.ArkBizInfo)
	for _, info := range bizInfos {
		bizRuntimeInfos[b.modelUtils.GetBizIdentityFromBizInfo(&info)] = &info
	}
	// bizName -> container status
	containerStatuses := make(map[string]*corev1.ContainerStatus)
	/**
	todo: if arklet return installed timestamp, we can submit corresponding event and start time accordingly
	      for now, we can just keep them empty
	if further info is provided, we can set the time accordingly
	failedTime would be the earliest time of the failed container
	successTime would be the latest time of the success container
	startTime would be the earliest time of the all container
	*/
	for _, bizModel := range bizModels {
		info := bizRuntimeInfos[b.modelUtils.GetBizIdentityFromBizModel(bizModel)]
		containerStatus := b.modelUtils.TranslateArkBizInfoToV1ContainerStatus(bizModel, info, b.bas.lastStatus)
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
		podStatus.Phase = corev1.PodFailed
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

func (b *BaseProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, opts api.ContainerLogOpts) (io.ReadCloser, error) {
	// todo: implement this by using the port to get the logs
	return nil, nil
}

func (b *BaseProvider) RunInContainer(ctx context.Context, namespace, podName, containerName string, cmd []string, attach api.AttachIO) error {
	panic("koupleless java virtual base does not support run")
}

func (b *BaseProvider) AttachToContainer(ctx context.Context, namespace, podName, containerName string, attach api.AttachIO) error {
	panic("koupleless java virtual base does not support attach")
}

func (b *BaseProvider) GetStatsSummary(ctx context.Context) (*statsv1alpha1.Summary, error) {
	// TODO implement later, with node status and pod status summary
	pods, _ := b.GetPods(ctx)
	podsSummary := make([]statsv1alpha1.PodStats, len(pods))
	for index, pod := range pods {
		podsSummary[index] = b.modelUtils.TranslatePodToSummaryPodStats(pod)
	}
	return &statsv1alpha1.Summary{
		Node: statsv1alpha1.NodeStats{
			NodeName:         "",
			SystemContainers: nil,
			StartTime:        metav1.Time{},
			CPU:              nil,
			Memory:           nil,
			Network:          nil,
			Fs:               nil,
			Runtime:          nil,
			Rlimit:           nil,
		},
		Pods: podsSummary,
	}, nil
}

func (b *BaseProvider) GetMetricsResource(ctx context.Context) ([]*io_prometheus_client.MetricFamily, error) {
	// todo: implement me
	return make([]*io_prometheus_client.MetricFamily, 0), nil
}

func (b *BaseProvider) PortForward(ctx context.Context, namespace, pod string, port int32, stream io.ReadWriteCloser) error {
	panic("koupleless java virtual base does not support port forward")
}
