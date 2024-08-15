package base_register_controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/trace"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/vnode/base_node"
	errpkg "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/informers"
	informer "k8s.io/client-go/informers/core/v1"
	lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"time"
)

var (
	vkNum            prometheus.Gauge
	podScheduleDelay prometheus.Gauge
	podScheduleNum   prometheus.Counter
	podDeleteNum     prometheus.Counter
)

func init() {
	vkNum = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "vk_manager",
		Name:      "vk_num",
		Help:      "VK num",
	})
	podScheduleDelay = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "vk_manager",
		Name:      "pod_schedule_delay",
		Help:      "Pod schedule delay",
	})
	podScheduleNum = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "vk_manager",
		Name:      "pod_schedule_num",
		Help:      "Pod schedule num",
	})
	podDeleteNum = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "vk_manager",
		Name:      "pod_delete_num",
		Help:      "Pod delete num",
	})
}

type BaseRegisterController struct {
	config *BuildBaseRegisterControllerConfig

	tunnels []tunnel.Tunnel

	podLister   lister.PodLister
	podInformer informer.PodInformer
	done        chan struct{}
	ready       chan struct{}

	err error

	runtimeInfoStore *RuntimeInfoStore
}

func NewBaseRegisterController(config *BuildBaseRegisterControllerConfig) (*BaseRegisterController, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	if len(config.Tunnels) == 0 {
		return nil, errors.New("config must have at least one tunnel provider")
	}

	return &BaseRegisterController{
		config:           config,
		tunnels:          config.Tunnels,
		done:             make(chan struct{}),
		ready:            make(chan struct{}),
		runtimeInfoStore: NewRuntimeInfoStore(),
	}, nil
}

func (brc *BaseRegisterController) Run(ctx context.Context) {
	var err error

	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel()
		brc.err = err
		close(brc.done)
	}()

	// init all tunnels
	for _, t := range brc.tunnels {
		t.RegisterCallback(brc.onNodeDiscovered, brc.onNodeStatusDataArrived, brc.onQueryAllContainerStatusDataArrived, brc.onInstallResponseArrived, brc.onUninstallResponseArrived)
	}

	requirement, _ := labels.NewRequirement(model.LabelKeyOfScheduleAnythingComponent, selection.In, []string{model.ComponentVPod})

	podInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		brc.config.K8SConfig.KubeClient,
		brc.config.K8SConfig.InformerSyncPeriod,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			// filter all module pods
			options.LabelSelector = labels.NewSelector().Add(*requirement).String()
		}),
	)

	podInformer := podInformerFactory.Core().V1().Pods()
	podLister := podInformer.Lister()
	podInformerFactory.Start(ctx.Done())

	// Wait for the caches to be synced *before* starting to do work.
	if ok := cache.WaitForCacheSync(ctx.Done(), podInformer.Informer().HasSynced); !ok {
		err = errors.New("failed to wait for caches to sync")
		return
	}

	brc.podLister = podLister
	brc.podInformer = podInformer

	// wait for all tunnel to be ready
	utils.CheckAndFinallyCall(func() bool {
		for _, t := range brc.tunnels {
			if !t.Ready() {
				return false
			}
		}
		return true
	}, time.Minute, time.Second, func() {
		log.G(ctx).Info("all tunnels are ready")
	}, func() {
		log.G(ctx).Error("waiting for all tunnels to be ready timeout")
		cancel()
	})

	// restart virtual kubelet for previous node
	brc.discoverPreviousNode(ctx)

	var eventHandler cache.ResourceEventHandler = cache.ResourceEventHandlerFuncs{
		AddFunc:    brc.podAddHandler,
		UpdateFunc: brc.podUpdateHandler,
		DeleteFunc: brc.podDeleteHandler,
	}

	_, err = podInformer.Informer().AddEventHandler(eventHandler)
	if err != nil {
		return
	}

	go utils.TimedTaskWithInterval(ctx, time.Second, brc.checkAndDeleteOfflineBase)

	close(brc.ready)
	log.G(ctx).Info("Base register controller ready")

	select {
	case <-brc.done:
	case <-ctx.Done():
	}
}

func (brc *BaseRegisterController) discoverPreviousNode(ctx context.Context) {
	componentRequirement, _ := labels.NewRequirement(model.LabelKeyOfScheduleAnythingComponent, selection.In, []string{model.ComponentVNode})
	envRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{brc.config.Env})

	nodeList, err := brc.config.K8SConfig.KubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labels.NewSelector().Add(*componentRequirement, *envRequirement).String(),
	})
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to list nodes")
		return
	}

	tmpTunnelMap := make(map[string]tunnel.Tunnel)

	for _, t := range brc.tunnels {
		tmpTunnelMap[t.Key()] = t
	}

	for _, node := range nodeList.Items {
		tunnelKey := node.Labels[model.LabelKeyOfVnodeTunnel]
		t, ok := tmpTunnelMap[tunnelKey]
		if !ok {
			continue
		}
		// base online message
		nodeIP := "127.0.0.1"
		nodeHostname := node.Name
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIP = addr.Address
			} else if addr.Type == corev1.NodeHostName {
				nodeHostname = addr.Address
			}
		}
		go brc.startVirtualKubelet(utils.ExtractBaseIDFromNodeName(node.Name), model.NodeInfo{
			Metadata: model.NodeMetadata{
				Name:    node.Labels[model.LabelKeyOfVNodeName],
				Status:  model.NodeStatusActivated,
				Version: node.Labels[model.LabelKeyOfVNodeVersion],
			},
			NetworkInfo: model.NetworkInfo{
				NodeIP:   nodeIP,
				HostName: nodeHostname,
			},
		}, t)
	}
}

func (brc *BaseRegisterController) onNodeDiscovered(baseID string, data model.NodeInfo, t tunnel.Tunnel) {
	if data.Metadata.Status == model.NodeStatusActivated {
		// base online message
		go brc.startVirtualKubelet(baseID, data, t)
	} else {
		// base offline message
		brc.shutdownVirtualKubelet(baseID)
	}
}

func (brc *BaseRegisterController) onNodeStatusDataArrived(baseID string, data model.NodeStatusData) {
	javaBaseNode := brc.runtimeInfoStore.GetBaseNode(baseID)
	if javaBaseNode == nil {
		return
	}
	brc.runtimeInfoStore.BaseMsgArrived(baseID)

	javaBaseNode.SyncNodeStatus(data)
}

func (brc *BaseRegisterController) onQueryAllContainerStatusDataArrived(baseID string, data []model.ContainerStatusData) {
	javaBaseNode := brc.runtimeInfoStore.GetBaseNode(baseID)
	if javaBaseNode == nil {
		return
	}
	brc.runtimeInfoStore.BaseMsgArrived(baseID)

	javaBaseNode.SyncAllContainerInfo(data)
}

func (brc *BaseRegisterController) onInstallResponseArrived(baseID string, data model.ContainerOperationResponseData) {
	javaBaseNode := brc.runtimeInfoStore.GetBaseNode(baseID)
	if javaBaseNode == nil {
		return
	}
	brc.runtimeInfoStore.BaseMsgArrived(baseID)
	// TODO
}

func (brc *BaseRegisterController) onUninstallResponseArrived(baseID string, data model.ContainerOperationResponseData) {
	javaBaseNode := brc.runtimeInfoStore.GetBaseNode(baseID)
	if javaBaseNode == nil {
		return
	}
	brc.runtimeInfoStore.BaseMsgArrived(baseID)
	// TODO
}

func (brc *BaseRegisterController) podAddHandler(pod interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "AddFunc")
	defer span.End()
	podFromKubernetes, ok := pod.(*corev1.Pod)
	if !ok {
		return
	}
	podScheduleNum.Inc()
	podScheduleDelay.Add(float64(time.Now().Sub(podFromKubernetes.CreationTimestamp.Time).Milliseconds()))
	nodeName := podFromKubernetes.Spec.NodeName
	// check node name in local storage
	kn := brc.runtimeInfoStore.GetBaseNodeByNodeName(nodeName)
	if kn == nil {
		// node not exist, invalid add req
		return
	}

	key, err := cache.MetaNamespaceKeyFunc(podFromKubernetes)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("Error getting key for pod %v", podFromKubernetes)
		return
	}
	ctx = span.WithField(ctx, "pod_key", key)

	kn.PodStore(key, podFromKubernetes)
	kn.SyncPodsFromKubernetesEnqueue(ctx, key)
}

func (brc *BaseRegisterController) podUpdateHandler(oldObj, newObj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "UpdateFunc")
	defer span.End()

	// Create a copy of the old and new pod objects so we don't mutate the cache.
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	nodeName := newPod.Spec.NodeName
	// check node name in local storage
	kn := brc.runtimeInfoStore.GetBaseNodeByNodeName(nodeName)
	if kn == nil {
		// node not exist, invalid add req
		return
	}

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	key, err := cache.MetaNamespaceKeyFunc(newPod)
	if err != nil {
		log.G(ctx).WithError(err).Errorf("Error getting key for pod %v", newPod)
		return
	}
	ctx = span.WithField(ctx, "key", key)
	obj, ok := kn.LoadPodFromController(key)
	isNewPod := false
	if !ok {
		kn.PodStore(key, newPod)
		isNewPod = true
	} else {
		kn.CheckAndUpdatePod(ctx, key, obj, newPod)
	}

	if isNewPod || podShouldEnqueue(oldPod, newPod) {
		kn.SyncPodsFromKubernetesEnqueue(ctx, key)
	}
}

func (brc *BaseRegisterController) podDeleteHandler(pod interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "DeleteFunc")
	defer span.End()

	if key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(pod); err != nil {
		log.G(ctx).Error(err)
	} else {
		podFromKubernetes, ok := pod.(*corev1.Pod)
		if !ok {
			return
		}
		podDeleteNum.Inc()

		nodeName := podFromKubernetes.Spec.NodeName
		// check node name in local storage
		kn := brc.runtimeInfoStore.GetBaseNodeByNodeName(nodeName)
		if kn == nil {
			// node not exist, invalid add req
			return
		}
		ctx = span.WithField(ctx, "key", key)
		kn.DeletePod(key)
		kn.SyncPodsFromKubernetesEnqueue(ctx, key)
		// If this pod was in the deletion queue, forget about it
		key = fmt.Sprintf("%v/%v", key, podFromKubernetes.UID)
		kn.DeletePodsFromKubernetesForget(ctx, key)
	}
}

func (brc *BaseRegisterController) checkAndDeleteOfflineBase(_ context.Context) {
	offlineBase := brc.runtimeInfoStore.GetOfflineBases(1000 * 20)
	for _, baseID := range offlineBase {
		brc.shutdownVirtualKubelet(baseID)
	}
}

func (brc *BaseRegisterController) Done() chan struct{} {
	return brc.done
}

func (brc *BaseRegisterController) Ready() chan struct{} {
	return brc.ready
}

func (brc *BaseRegisterController) WaitReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	select {
	case <-brc.Ready():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (brc *BaseRegisterController) Err() error {
	return brc.err
}

func (brc *BaseRegisterController) startVirtualKubelet(baseID string, initData model.NodeInfo, t tunnel.Tunnel) {
	var err error
	// first apply for local lock
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "baseID", baseID))
	defer cancel()
	if initData.NetworkInfo.NodeIP == "" {
		initData.NetworkInfo.NodeIP = "127.0.0.1"
	}

	// TODO apply for distributed lock in future, to support sharding, after getting lock, create node
	err = brc.runtimeInfoStore.PutBaseIDNX(baseID)
	if err != nil {
		// already exist, return
		return
	}
	vkNum.Inc()
	defer func() {
		vkNum.Dec()
		// delete from local storage
		brc.runtimeInfoStore.DeleteBaseNode(baseID)
		if err != nil {
			logrus.Infof("koupleless node exit: %s, err: %v", baseID, err)
		} else {
			logrus.Infof("koupleless node exit %s, base exit", baseID)
		}
	}()

	bn, err := base_node.NewBaseNode(&base_node.BuildBaseNodeConfig{
		KubeClient:   brc.config.K8SConfig.KubeClient,
		PodLister:    brc.podLister,
		PodInformer:  brc.podInformer,
		Tunnel:       t,
		BaseID:       baseID,
		Env:          brc.config.Env,
		NodeIP:       initData.NetworkInfo.NodeIP,
		NodeHostname: initData.NetworkInfo.HostName,
		NodeName:     initData.Metadata.Name,
		NodeVersion:  initData.Metadata.Version,
	})
	if err != nil {
		err = errpkg.Wrap(err, "Error creating base vnode")
		return
	}

	go bn.Run(ctx)

	if err = bn.WaitReady(ctx, time.Minute); err != nil {
		err = errpkg.Wrap(err, "Error waiting base vnode ready")
		return
	}

	brc.runtimeInfoStore.PutBaseNode(baseID, bn)

	logrus.Infof("vnode running: %s", baseID)

	// record first msg arrived time
	brc.runtimeInfoStore.BaseMsgArrived(baseID)

	select {
	case <-bn.Done():
		err = bn.Err()
	case <-ctx.Done():
		err = ctx.Err()
	}
}

func (brc *BaseRegisterController) shutdownVirtualKubelet(baseID string) {
	baseNode := brc.runtimeInfoStore.GetBaseNode(baseID)
	if baseNode == nil {
		// exited
		return
	}
	baseNode.Exit()
}
