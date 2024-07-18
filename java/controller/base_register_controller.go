package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/common/trace"
	"github.com/koupleless/virtual-kubelet/java/common"
	"github.com/koupleless/virtual-kubelet/java/model"
	"github.com/koupleless/virtual-kubelet/java/pod/node"
	"github.com/koupleless/virtual-kubelet/node/nodeutil"
	errpkg "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"time"
)

type BaseRegisterController struct {
	config *model.BuildBaseRegisterControllerConfig

	mqttClient *mqtt.Client
	kubeClient *kubernetes.Clientset
	podLister  v1.PodLister
	done       chan struct{}
	ready      chan struct{}

	err error

	localStore *RuntimeInfoStore
}

func NewBaseRegisterController(config *model.BuildBaseRegisterControllerConfig) (*BaseRegisterController, error) {
	return &BaseRegisterController{
		config:     config,
		done:       make(chan struct{}),
		ready:      make(chan struct{}),
		localStore: NewRuntimeInfoStore(),
	}, nil
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
	nodeName := podFromKubernetes.Spec.NodeName
	// check node name in local storage
	kn := brc.localStore.GetBaseNodeByNodeID(nodeName)
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
	kn := brc.localStore.GetBaseNodeByNodeID(nodeName)
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

		nodeName := podFromKubernetes.Spec.NodeName
		// check node name in local storage
		kn := brc.localStore.GetBaseNodeByNodeID(nodeName)
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

func (brc *BaseRegisterController) Run(ctx context.Context) {
	var err error

	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel()
		brc.err = err
		close(brc.done)
	}()

	clientSet, err := nodeutil.ClientsetFromEnv(brc.config.K8SConfig.KubeConfigPath)
	if err != nil {
		return
	}

	requirement, err := labels.NewRequirement("module-controller.koupleless.io/component", selection.In, []string{"module"})
	if err != nil {
		return
	}

	podInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		clientSet,
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

	brc.kubeClient = clientSet
	brc.podLister = podLister

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

	brc.config.MqttConfig.OnConnectHandler = func(client paho.Client) {
		log.G(ctx).Info("Connected")
		reader := client.OptionsReader()
		log.G(ctx).Info("Connect options: ", reader.ClientID())
		client.Subscribe(BaseHeartBeatTopic, mqtt.Qos1, brc.heartBeatMsgCallback)
		client.Subscribe(BaseHealthTopic, mqtt.Qos1, brc.healthMsgCallback)
		client.Subscribe(BaseBizTopic, mqtt.Qos1, brc.bizMsgCallback)
		select {
		case <-brc.ready:
		default:
			close(brc.ready)
		}
	}
	mqttClient, err := mqtt.NewMqttClient(brc.config.MqttConfig)
	if err != nil {
		brc.err = err
		return
	}
	if mqttClient == nil {
		brc.err = errors.New("mqtt client is nil")
		return
	}
	brc.mqttClient = mqttClient

	go common.TimedTaskWithInterval(ctx, time.Second, brc.checkAndDeleteOfflineBase)

	select {
	case <-brc.done:
	case <-ctx.Done():
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

func (brc *BaseRegisterController) Err() error {
	return brc.err
}

func (brc *BaseRegisterController) startVirtualKubelet(baseID string, initData HeartBeatData) {
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
	defer func() {
		// delete from local storage
		brc.localStore.DeleteKouplelessNode(baseID)
		if err != nil {
			logrus.Infof("koupleless node exit: %s, err: %v", baseID, err)
		} else {
			logrus.Infof("koupleless node exit %s, base exit", baseID)
		}
	}()

	kn, err := node.NewKouplelessNode(&model.BuildKouplelessNodeConfig{
		KubeClient: brc.kubeClient,
		PodLister:  brc.podLister,
		MqttClient: brc.mqttClient,
		NodeID:     baseID,
		NodeIP:     initData.NetworkInfo.LocalIP,
		TechStack:  "java",
		BizName:    initData.MasterBizInfo.BizName,
		BizVersion: initData.MasterBizInfo.BizVersion,
	})
	if err != nil {
		err = errpkg.Wrap(err, "Error creating Koupleless node")
		return
	}

	go kn.Run(ctx)

	if err = kn.WaitReady(ctx, time.Minute); err != nil {
		err = errpkg.Wrap(err, "Error waiting Koupleless node ready")
		return
	}

	brc.localStore.PutKouplelessNode(baseID, kn)

	logrus.Infof("koupleless node running: %s", baseID)

	// record first msg arrived time
	brc.localStore.BaseMsgArrived(baseID)

	select {
	case <-kn.Done():
		err = kn.Err()
	case <-ctx.Done():
		err = ctx.Err()
	}
}

func (brc *BaseRegisterController) shutdownVirtualKubelet(baseID string) {
	kouplelessNode := brc.localStore.GetKouplelessNode(baseID)
	if kouplelessNode == nil {
		// exited
		return
	}
	close(kouplelessNode.BaseBizExitChan)
}

func (brc *BaseRegisterController) heartBeatMsgCallback(_ paho.Client, msg paho.Message) {
	if msg.Qos() > 0 {
		defer msg.Ack()
	}
	baseID := getBaseIDFromTopic(msg.Topic())
	if baseID == "" {
		logrus.Error("Received non device heart beat msg from topic: ", msg.Topic())
		return
	}
	var data ArkMqttMsg[HeartBeatData]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling heart beat data: %v", err)
		return
	}
	go func() {
		if expired(data.PublishTimestamp, 1000*10) {
			return
		}
		if data.Data.MasterBizInfo.BizState == "ACTIVATED" {
			// base online message
			brc.startVirtualKubelet(baseID, data.Data)
		} else {
			// base offline message
			brc.shutdownVirtualKubelet(baseID)
		}

	}()
}

func (brc *BaseRegisterController) healthMsgCallback(_ paho.Client, msg paho.Message) {
	if msg.Qos() > 0 {
		defer msg.Ack()
	}
	baseID := getBaseIDFromTopic(msg.Topic())
	if baseID == "" {
		return
	}
	var data ArkMqttMsg[ark.HealthResponse]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling health response: %v", err)
		return
	}

	go func() {
		if expired(data.PublishTimestamp, 1000*10) {
			return
		}
		if data.Data.Code != "SUCCESS" {
			return
		}
		kouplelessNode := brc.localStore.GetKouplelessNode(baseID)
		if kouplelessNode == nil {
			return
		}
		brc.localStore.BaseMsgArrived(baseID)

		select {
		case kouplelessNode.BaseHealthInfoChan <- data.Data.Data.HealthData:
		default:
		}
	}()
}

func (brc *BaseRegisterController) bizMsgCallback(_ paho.Client, msg paho.Message) {
	if msg.Qos() > 0 {
		defer msg.Ack()
	}
	baseID := getBaseIDFromTopic(msg.Topic())
	if baseID == "" {
		return
	}
	var data ArkMqttMsg[ark.QueryAllArkBizResponse]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling biz response: %v", err)
		return
	}

	go func() {
		if expired(data.PublishTimestamp, 1000*10) {
			return
		}
		if data.Data.Code != "SUCCESS" {
			return
		}
		kouplelessNode := brc.localStore.GetKouplelessNode(baseID)
		if kouplelessNode == nil {
			return
		}
		brc.localStore.BaseMsgArrived(baseID)
		select {
		case kouplelessNode.BaseBizInfoChan <- data.Data.Data:
		default:
		}
	}()
}
