package vnode_controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/trace"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/vnode"
	errpkg "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

type VNodeController struct {
	clientID string

	env string

	vPodIdentity string

	tunnels []tunnel.Tunnel

	client client.Client
	cache  cache.Cache

	ready chan struct{}

	runtimeInfoStore *RuntimeInfoStore
}

func (brc *VNodeController) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// do nothing here
	//<-brc.ready
	//pod := &corev1.Pod{}
	//err := brc.client.Get(ctx, types.NamespacedName{
	//	Namespace: request.Namespace,
	//	Name:      request.Name,
	//}, pod)
	//if err != nil {
	//	if k8sErr.IsNotFound(err) {
	//		log.G(ctx).WithField("vpodName", pod.Name).Info("vpod has been deleted")
	//		return reconcile.Result{}, nil
	//	}
	//	log.G(ctx).WithField("vpodName", pod.Name).WithError(err).Info("fail to get vpod")
	//	return reconcile.Result{
	//		Requeue:      true,
	//		RequeueAfter: time.Second * 5,
	//	}, nil
	//}

	return reconcile.Result{}, nil
}

func NewVNodeController(config *model.BuildVNodeControllerConfig, tunnels []tunnel.Tunnel) (*VNodeController, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	if len(tunnels) == 0 {
		return nil, errors.New("config must have at least one tunnel provider")
	}

	if config.VPodIdentity == "" {
		return nil, errors.New("config must set vpod identity")
	}

	return &VNodeController{
		clientID:         config.ClientID,
		env:              config.Env,
		vPodIdentity:     config.VPodIdentity,
		tunnels:          tunnels,
		runtimeInfoStore: NewRuntimeInfoStore(),
		ready:            make(chan struct{}),
	}, nil
}

func (brc *VNodeController) SetupWithManager(ctx context.Context, mgr manager.Manager) (err error) {
	// init all tunnels
	for _, t := range brc.tunnels {
		t.RegisterCallback(brc.onNodeDiscovered, brc.onNodeStatusDataArrived, brc.onQueryAllContainerStatusDataArrived, brc.onContainerStatusChanged)
	}

	brc.client = mgr.GetClient()
	brc.cache = mgr.GetCache()

	log.G(ctx).Info("Setting up register controller")

	c, err := controller.New("vnode-controller", mgr, controller.Options{
		Reconciler: brc,
	})
	if err != nil {
		log.G(ctx).Error(err, "unable to set up vnode controller")
		return err
	}

	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{brc.vPodIdentity})

	podHandler := handler.TypedFuncs[*corev1.Pod, reconcile.Request]{
		CreateFunc: func(ctx context.Context, e event.TypedCreateEvent[*corev1.Pod], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			<-brc.ready
			brc.podCreateHandler(ctx, e.Object)
		},
		UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[*corev1.Pod], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			<-brc.ready
			brc.podUpdateHandler(ctx, e.ObjectOld, e.ObjectNew)
		},
		DeleteFunc: func(ctx context.Context, e event.TypedDeleteEvent[*corev1.Pod], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			<-brc.ready
			brc.podDeleteHandler(ctx, e.Object)
		},
		GenericFunc: func(ctx context.Context, e event.TypedGenericEvent[*corev1.Pod], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			<-brc.ready
			log.G(ctx).Warn("GenericFunc called")
		},
	}

	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &podHandler, &VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
	})); err != nil {
		log.G(ctx).WithError(err).Error("unable to watch Pods")
		return err
	}

	nodeHandler := handler.TypedFuncs[*corev1.Node, reconcile.Request]{
		CreateFunc: func(ctx context.Context, e event.TypedCreateEvent[*corev1.Node], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			log.G(ctx).Info("vnode created ", e.Object.Name)
		},
		UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[*corev1.Node], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		},
		DeleteFunc: func(ctx context.Context, e event.TypedDeleteEvent[*corev1.Node], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			log.G(ctx).Info("vnode deleted ", e.Object.Name)
			brc.shutdownVNode(utils.ExtractNodeIDFromNodeName(e.Object.Name))
		},
		GenericFunc: func(ctx context.Context, e event.TypedGenericEvent[*corev1.Node], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			log.G(ctx).Info("vnode generic ", e.Object.Name)
		},
	}

	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	envRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{brc.env})

	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Node{}, &nodeHandler, &VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement, *envRequirement),
	})); err != nil {
		log.G(ctx).WithError(err).Error("unable to watch VNodes")
		return err
	}

	go func() {
		// wait for all tunnel to be ready
		utils.CheckAndFinallyCall(context.Background(), func() bool {
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
		})

		syncd := brc.cache.WaitForCacheSync(ctx)
		if syncd {
			// restart virtual kubelet for previous node
			componentRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
			envRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{brc.env})

			nodeList := &corev1.NodeList{}
			err = brc.client.List(ctx, nodeList, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*componentRequirement, *envRequirement),
			})
			if err != nil {
				log.G(ctx).WithError(err).Error("failed to list nodes")
				return
			}
			brc.discoverPreviousNodes(ctx, nodeList)
			// sync pods from kubernetes
			podComponentRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{brc.vPodIdentity})

			podList := &corev1.PodList{}
			err = brc.client.List(ctx, podList, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*podComponentRequirement),
			})
			if err != nil {
				log.G(ctx).WithError(err).Error("failed to list pods")
				return
			}
			brc.discoverPreviousPods(ctx, podList)
			close(brc.ready)
		} else {
			log.G(ctx).Error("cache sync failed")
		}
	}()

	log.G(ctx).Info("register controller ready")

	go utils.TimedTaskWithInterval(ctx, time.Second, brc.checkAndModifyOfflineNode)

	return nil
}

func (brc *VNodeController) discoverPreviousNodes(ctx context.Context, nodeList *corev1.NodeList) {
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
		nodeIP := "127.0.0.1"
		nodeHostname := node.Name
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIP = addr.Address
			} else if addr.Type == corev1.NodeHostName {
				nodeHostname = addr.Address
			}
		}
		brc.startVNode(utils.ExtractNodeIDFromNodeName(node.Name), model.NodeInfo{
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

func (brc *VNodeController) discoverPreviousPods(ctx context.Context, podList *corev1.PodList) {
	for _, pod := range podList.Items {
		nodeName := pod.Spec.NodeName
		// check node name in local storage
		vn := brc.runtimeInfoStore.GetVNodeByNodeName(nodeName)
		if vn == nil {
			// node not exist
			continue
		}

		key := utils.GetPodKey(&pod)
		vn.PodStore(key, &pod)
		// sync container states
		containerNameToStatus := make(map[string]corev1.ContainerStatus)
		for _, containerStatus := range pod.Status.ContainerStatuses {
			containerNameToStatus[containerStatus.Name] = containerStatus
		}

		for _, container := range pod.Spec.Containers {
			containerStatus, has := containerNameToStatus[container.Name]
			if has && containerStatus.State.Running != nil {
				// means container has been ready before start
				vn.InitContainerInfo(model.ContainerStatusData{
					Key:        vn.Tunnel.GetContainerUniqueKey(key, &container),
					Name:       container.Name,
					PodKey:     key,
					State:      model.ContainerStateActivated,
					ChangeTime: containerStatus.State.Running.StartedAt.Time,
				})
			}
		}

		vn.SyncPodsFromKubernetesEnqueue(ctx, key)
	}
}

func (brc *VNodeController) onNodeDiscovered(nodeID string, data model.NodeInfo, t tunnel.Tunnel) {
	if data.Metadata.Status == model.NodeStatusActivated {
		brc.startVNode(nodeID, data, t)
	} else {
		brc.shutdownVNode(nodeID)
	}
}

func (brc *VNodeController) onNodeStatusDataArrived(nodeID string, data model.NodeStatusData) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		return
	}
	brc.runtimeInfoStore.NodeMsgArrived(nodeID)

	vNode.SyncNodeStatus(data)
}

func (brc *VNodeController) onQueryAllContainerStatusDataArrived(nodeID string, data []model.ContainerStatusData) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		return
	}
	brc.runtimeInfoStore.NodeMsgArrived(nodeID)

	vNode.SyncAllContainerInfo(context.TODO(), data)
}

func (brc *VNodeController) onContainerStatusChanged(nodeID string, data model.ContainerStatusData) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		return
	}
	brc.runtimeInfoStore.NodeMsgArrived(nodeID)

	vNode.SyncSingleContainerInfo(context.TODO(), data)
}

func (brc *VNodeController) podCreateHandler(ctx context.Context, podFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "CreateFunc")
	defer span.End()

	nodeName := podFromKubernetes.Spec.NodeName
	// check node name in local storage
	vn := brc.runtimeInfoStore.GetVNodeByNodeName(nodeName)
	if vn == nil {
		// node not exist, invalid add req
		return
	}

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	key := utils.GetPodKey(podFromKubernetes)
	ctx = span.WithField(ctx, "key", key)
	vn.PodStore(key, podFromKubernetes)

	vn.SyncPodsFromKubernetesEnqueue(ctx, key)
}

func (brc *VNodeController) podUpdateHandler(ctx context.Context, oldPodFromKubernetes, newPodFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "UpdateFunc")
	defer span.End()

	nodeName := newPodFromKubernetes.Spec.NodeName
	// check node name in local storage
	vn := brc.runtimeInfoStore.GetVNodeByNodeName(nodeName)
	if vn == nil {
		// node not exist, invalid add req
		return
	}

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	key := utils.GetPodKey(newPodFromKubernetes)
	ctx = span.WithField(ctx, "key", key)
	obj, ok := vn.LoadPodFromController(key)
	isNewPod := false
	if !ok {
		vn.PodStore(key, newPodFromKubernetes)
		isNewPod = true
	} else {
		vn.CheckAndUpdatePod(ctx, key, obj, newPodFromKubernetes)
	}

	if isNewPod || podShouldEnqueue(oldPodFromKubernetes, newPodFromKubernetes) {
		vn.SyncPodsFromKubernetesEnqueue(ctx, key)
	}
}

func (brc *VNodeController) podDeleteHandler(ctx context.Context, podFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "DeleteFunc")
	defer span.End()

	key := utils.GetPodKey(podFromKubernetes)

	nodeName := podFromKubernetes.Spec.NodeName
	// check node name in local storage
	vn := brc.runtimeInfoStore.GetVNodeByNodeName(nodeName)
	if vn == nil {
		// node not exist, invalid add req
		return
	}
	ctx = span.WithField(ctx, "key", key)
	vn.SyncPodsFromKubernetesEnqueue(ctx, key)
	// If this pod was in the deletion queue, forget about it
	key = fmt.Sprintf("%v/%v", key, podFromKubernetes.UID)
	vn.DeletePodsFromKubernetesForget(ctx, key)
}

func (brc *VNodeController) checkAndModifyOfflineNode(_ context.Context) {
	offlineNodes := brc.runtimeInfoStore.GetOfflineNodes(1000 * 40)
	for _, nodeID := range offlineNodes {
		brc.onNodeStatusDataArrived(nodeID, model.NodeStatusData{
			CustomConditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "NodeOffline",
					Message: "It has been more than 20 seconds since the message from Node was received, please check",
				},
			},
		})
	}
}

func (brc *VNodeController) startVNode(nodeID string, initData model.NodeInfo, t tunnel.Tunnel) {
	var err error
	// first apply for local lock
	if initData.NetworkInfo.NodeIP == "" {
		initData.NetworkInfo.NodeIP = "127.0.0.1"
	}

	err = brc.runtimeInfoStore.PutVNodeIDNX(nodeID)
	if err != nil {
		// already exist, return
		return
	}

	defer func() {
		if err != nil {
			logrus.WithError(err).Errorf("failed to start node %s", nodeID)
		}
	}()

	vn, err := vnode.NewVNode(&model.BuildVNodeConfig{
		Client:       brc.client,
		KubeCache:    brc.cache,
		NodeID:       nodeID,
		Env:          brc.env,
		NodeIP:       initData.NetworkInfo.NodeIP,
		NodeHostname: initData.NetworkInfo.HostName,
		NodeName:     initData.Metadata.Name,
		NodeVersion:  initData.Metadata.Version,
		CustomTaints: initData.CustomTaints,
	}, t)
	if err != nil {
		err = errpkg.Wrap(err, "Error creating vnode")
		return
	}

	brc.runtimeInfoStore.PutVNode(nodeID, vn)

	vnCtx := context.WithValue(context.Background(), "nodeID", nodeID)
	vnCtx, vnCancel := context.WithCancel(vnCtx)

	go vn.Run(vnCtx)

	if err = vn.WaitReady(time.Minute); err != nil {
		err = errpkg.Wrap(err, "Error waiting vnode ready")
		vnCancel()
		return
	}

	t.OnNodeStart(vnCtx, nodeID)

	go utils.TimedTaskWithInterval(vnCtx, time.Second*9, func(ctx context.Context) {
		err = t.FetchHealthData(ctx, nodeID)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Failed to fetch node health info from %s", nodeID)
		}
	})

	go utils.TimedTaskWithInterval(vnCtx, time.Second*5, func(ctx context.Context) {
		err = t.QueryAllContainerStatusData(ctx, nodeID)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Failed to query containers info from %s", nodeID)
		}
	})

	// record first msg arrived time
	brc.runtimeInfoStore.NodeMsgArrived(nodeID)

	go func() {
		select {
		case <-vn.Done():
			logrus.WithError(vn.Err()).Infof("node exit %s", nodeID)
		case <-vnCtx.Done():
			logrus.WithError(vnCtx.Err()).Infof("node exit %s", nodeID)
		}
		vnCancel()
		brc.runtimeInfoStore.DeleteVNode(nodeID)
	}()
}

func (brc *VNodeController) shutdownVNode(nodeID string) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		// exited
		return
	}
	vNode.Shutdown()
}

func (brc *VNodeController) setVNodeUnready(nodeID string) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		// exited
		return
	}
	vNode.Shutdown()
}
