package base_register_controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/koupleless/arkctl/v1/service/ark"
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

	localStore *RuntimeInfoStore
}

func NewBaseRegisterController(config *BuildBaseRegisterControllerConfig) (*BaseRegisterController, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	if len(config.Tunnels) == 0 {
		return nil, errors.New("config must have at least one tunnel provider")
	}

	return &BaseRegisterController{
		config:     config,
		tunnels:    config.Tunnels,
		done:       make(chan struct{}),
		ready:      make(chan struct{}),
		localStore: NewRuntimeInfoStore(),
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

	// init all tunnels, and at least one t start successfully
	anyTunnelStarted := false
	for _, t := range brc.tunnels {
		err = t.Register(ctx, brc.config.ClientID, brc.config.Env, brc.onBaseDiscovered, brc.onHealthDataArrived, brc.onBizDataArrived, nil, nil)
		if err != nil {
			log.G(ctx).WithError(err).Error("failed to register tunnel:", t.Name())
			continue
		}

		anyTunnelStarted = true
	}

	if !anyTunnelStarted {
		err = errors.New("no tunnel has been started")
		return
	}

	requirement, _ := labels.NewRequirement(model.PodModuleControllerComponentLabelKey, selection.In, []string{model.ModuleControllerComponentModule})

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

	log.G(ctx).Info("Pod cache in-sync")

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

func (brc *BaseRegisterController) onBaseDiscovered(baseID string, data model.HeartBeatData, t tunnel.Tunnel) {
	if data.MasterBizInfo.BizState == "ACTIVATED" {
		// base online message
		go brc.startVirtualKubelet(baseID, data, t)
	} else {
		// base offline message
		brc.shutdownVirtualKubelet(baseID)
	}
}

func (brc *BaseRegisterController) onHealthDataArrived(baseID string, data ark.HealthData) {
	javaBaseNode := brc.localStore.GetBaseNode(baseID)
	if javaBaseNode == nil {
		return
	}
	brc.localStore.BaseMsgArrived(baseID)

	select {
	case javaBaseNode.BaseHealthInfoChan <- data:
	default:
	}
}

func (brc *BaseRegisterController) onBizDataArrived(baseID string, data []ark.ArkBizInfo) {
	javaBaseNode := brc.localStore.GetBaseNode(baseID)
	if javaBaseNode == nil {
		return
	}
	brc.localStore.BaseMsgArrived(baseID)
	select {
	case javaBaseNode.BaseBizInfoChan <- data:
	default:
	}
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
	kn := brc.localStore.GetBaseNodeByNodeName(nodeName)
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
	kn := brc.localStore.GetBaseNodeByNodeName(nodeName)
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
		kn := brc.localStore.GetBaseNodeByNodeName(nodeName)
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
	offlineBase := brc.localStore.GetOfflineBases(1000 * 20)
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

func (brc *BaseRegisterController) startVirtualKubelet(baseID string, initData model.HeartBeatData, t tunnel.Tunnel) {
	var err error
	// first apply for local lock
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "baseID", baseID))
	defer cancel()
	if initData.NetworkInfo.LocalIP == "" {
		initData.NetworkInfo.LocalIP = "127.0.0.1"
	}

	// TODO apply for distributed lock in future, to support sharding, after getting lock, create node
	err = brc.localStore.PutBaseIDNX(baseID)
	if err != nil {
		// already exist, return
		return
	}
	vkNum.Inc()
	defer func() {
		vkNum.Dec()
		// delete from local storage
		brc.localStore.DeleteBaseNode(baseID)
		if err != nil {
			logrus.Infof("koupleless node exit: %s, err: %v", baseID, err)
		} else {
			logrus.Infof("koupleless node exit %s, base exit", baseID)
		}
	}()

	bn, err := base_node.NewBaseNode(&base_node.BuildBaseNodeConfig{
		KubeClient:  brc.config.K8SConfig.KubeClient,
		PodLister:   brc.podLister,
		PodInformer: brc.podInformer,
		Tunnel:      t,
		BaseID:      baseID,
		Env:         brc.config.Env,
		NodeIP:      initData.NetworkInfo.LocalIP,
		// TODO support read from base heart beat data
		TechStack:  "java",
		BizName:    initData.MasterBizInfo.BizName,
		BizVersion: initData.MasterBizInfo.BizVersion,
	})
	if err != nil {
		err = errpkg.Wrap(err, "Error creating Koupleless node")
		return
	}

	go bn.Run(ctx)

	if err = bn.WaitReady(ctx, time.Minute); err != nil {
		err = errpkg.Wrap(err, "Error waiting Koupleless node ready")
		return
	}

	brc.localStore.PutBaseNode(baseID, bn)

	logrus.Infof("koupleless node running: %s", baseID)

	// record first msg arrived time
	brc.localStore.BaseMsgArrived(baseID)

	select {
	case <-bn.Done():
		err = bn.Err()
	case <-ctx.Done():
		err = ctx.Err()
	}
}

func (brc *BaseRegisterController) shutdownVirtualKubelet(baseID string) {
	baseNode := brc.localStore.GetBaseNode(baseID)
	if baseNode == nil {
		// exited
		return
	}
	baseNode.Exit()
}
