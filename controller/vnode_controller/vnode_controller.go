package vnode_controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/trace"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/controller/vnode_controller/predicates"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/vnode"
	errpkg "github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/coordination/v1"
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

	isCluster bool

	workloadMaxLevel int

	tunnels []tunnel.Tunnel

	client client.Client
	cache  cache.Cache

	ready chan struct{}

	runtimeInfoStore *RuntimeInfoStore
}

func (brc *VNodeController) Reconcile(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
	// do nothing here
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

	if config.IsCluster && config.WorkloadMaxLevel == 0 {
		config.WorkloadMaxLevel = 3
	}

	return &VNodeController{
		clientID:         config.ClientID,
		env:              config.Env,
		vPodIdentity:     config.VPodIdentity,
		isCluster:        config.IsCluster,
		workloadMaxLevel: config.WorkloadMaxLevel,
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

	if err = mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, "spec.nodeName", func(rawObj client.Object) []string {
		pod := rawObj.(*corev1.Pod)
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return err
	}

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
	}

	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &podHandler, &predicates.VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
	})); err != nil {
		log.G(ctx).WithError(err).Error("unable to watch Pods")
		return err
	}

	nodeHandler := handler.TypedFuncs[*corev1.Node, reconcile.Request]{
		CreateFunc: func(ctx context.Context, e event.TypedCreateEvent[*corev1.Node], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			log.G(ctx).Info("vnode created ", e.Object.Name)
			brc.runtimeInfoStore.PutNode(e.Object.Name)
		},
		UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[*corev1.Node], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		},
		DeleteFunc: func(ctx context.Context, e event.TypedDeleteEvent[*corev1.Node], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			log.G(ctx).Info("vnode deleted ", e.Object.Name)
			brc.runtimeInfoStore.DeleteNode(e.Object.Name)
			brc.shutdownVNode(utils.ExtractNodeIDFromNodeName(e.Object.Name))
		},
	}

	vnodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	envRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{brc.env})

	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Node{}, &nodeHandler, &predicates.VNodePredicate{
		VNodeLabelSelector: labels.NewSelector().Add(*vnodeRequirement, *envRequirement),
	})); err != nil {
		log.G(ctx).WithError(err).Error("unable to watch VNodes")
		return err
	}

	leaseHandler := handler.TypedFuncs[*v1.Lease, reconcile.Request]{
		CreateFunc: func(ctx context.Context, e event.TypedCreateEvent[*v1.Lease], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			brc.runtimeInfoStore.PutVNodeLeaseLatestUpdateTime(e.Object.Name, e.Object.Spec.RenewTime.Time)
		},
		UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[*v1.Lease], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			// refresh the latest update time of node
			brc.runtimeInfoStore.PutVNodeLeaseLatestUpdateTime(e.ObjectNew.Name, e.ObjectNew.Spec.RenewTime.Time)
		},
		DeleteFunc: func(ctx context.Context, e event.TypedDeleteEvent[*v1.Lease], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			log.G(ctx).Info("vnode lease deleted, wake vnode up", e.Object.Name)
			brc.wakeUpVNode(utils.ExtractNodeIDFromNodeName(e.Object.Name))
		},
	}

	vnodeLeaseRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNodeLease})

	if err = c.Watch(source.Kind(mgr.GetCache(), &v1.Lease{}, &leaseHandler, &predicates.VNodeLeasePredicate{
		LabelSelector: labels.NewSelector().Add(*vnodeLeaseRequirement, *envRequirement),
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

			nodeList := &corev1.NodeList{}
			err = brc.client.List(ctx, nodeList, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*componentRequirement, *envRequirement),
			})
			if err != nil {
				log.G(ctx).WithError(err).Error("failed to list nodes")
				return
			}

			for _, node := range nodeList.Items {
				brc.runtimeInfoStore.PutNode(node.Name)
			}

			brc.discoverPreviousNodes(nodeList)

			go utils.TimedTaskWithInterval(ctx, time.Millisecond*500, func(ctx context.Context) {
				outdatedVNodeNameList := brc.runtimeInfoStore.GetLeaseOutdatedVNodeName(time.Second * model.NodeLeaseDurationSeconds)
				for _, nodeName := range outdatedVNodeNameList {
					brc.wakeUpVNode(utils.ExtractNodeIDFromNodeName(nodeName))
				}
			})

			go utils.TimedTaskWithInterval(ctx, time.Second, func(ctx context.Context) {
				notReachableNodeInfos := brc.runtimeInfoStore.GetNotReachableNodeInfos(time.Second * model.NodeLeaseDurationSeconds)
				for _, nodeInfo := range notReachableNodeInfos {
					vNode := brc.runtimeInfoStore.GetVNode(nodeInfo.NodeID)
					if vNode == nil {
						continue
					}

					if vNode.IsLeader() {
						vNode.Tunnel.OnNodeNotReady(ctx, nodeInfo)
					}
				}
			})

			close(brc.ready)
		} else {
			log.G(ctx).Error("cache sync failed")
		}
	}()

	log.G(ctx).Info("register controller ready")

	return nil
}

func (brc *VNodeController) discoverPreviousNodes(nodeList *corev1.NodeList) {
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

func (brc *VNodeController) discoverPreviousPods(ctx context.Context, vn *vnode.VNode, podList *corev1.PodList) {
	for _, pod := range podList.Items {
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

	if vNode.IsLeader() {
		brc.runtimeInfoStore.NodeMsgArrived(nodeID)
		vNode.SyncNodeStatus(data)
	}
}

func (brc *VNodeController) onQueryAllContainerStatusDataArrived(nodeID string, data []model.ContainerStatusData) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		return
	}

	if vNode.IsLeader() {
		brc.runtimeInfoStore.NodeMsgArrived(nodeID)
		vNode.SyncContainerInfo(context.TODO(), data)
	}
}

func (brc *VNodeController) onContainerStatusChanged(nodeID string, data model.ContainerStatusData) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		return
	}

	if vNode.IsLeader() {
		brc.runtimeInfoStore.NodeMsgArrived(nodeID)
		vNode.SyncContainerInfo(context.TODO(), []model.ContainerStatusData{data})
	}
}

func (brc *VNodeController) podCreateHandler(ctx context.Context, podFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	nodeName := podFromKubernetes.Spec.NodeName
	// check node name in local storage
	vn := brc.runtimeInfoStore.GetVNodeByNodeName(nodeName)
	if vn == nil {
		// node not exist, invalid add req
		return
	}

	if !vn.IsLeader() {
		// not leader, just return
		return
	}

	ctx, span := trace.StartSpan(ctx, "CreateFunc")
	defer span.End()

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	key := utils.GetPodKey(podFromKubernetes)
	ctx = span.WithField(ctx, "key", key)
	vn.PodStore(key, podFromKubernetes)

	vn.SyncPodsFromKubernetesEnqueue(ctx, key)
}

func (brc *VNodeController) podUpdateHandler(ctx context.Context, oldPodFromKubernetes, newPodFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	nodeName := newPodFromKubernetes.Spec.NodeName
	// check node name in local storage
	vn := brc.runtimeInfoStore.GetVNodeByNodeName(nodeName)
	if vn == nil {
		// node not exist, invalid add req
		return
	}

	if !vn.IsLeader() {
		// not leader, just return
		return
	}
	ctx, span := trace.StartSpan(ctx, "UpdateFunc")
	defer span.End()

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

	nodeName := podFromKubernetes.Spec.NodeName
	// check node name in local storage
	vn := brc.runtimeInfoStore.GetVNodeByNodeName(nodeName)
	if vn == nil {
		// node not exist, invalid add req
		return
	}

	if !vn.IsLeader() {
		// not leader, just return
		return
	}
	ctx, span := trace.StartSpan(ctx, "DeleteFunc")
	defer span.End()

	key := utils.GetPodKey(podFromKubernetes)

	ctx = span.WithField(ctx, "key", key)
	vn.SyncPodsFromKubernetesEnqueue(ctx, key)
	// If this pod was in the deletion queue, forget about it
	key = fmt.Sprintf("%v/%v", key, podFromKubernetes.UID)
	vn.DeletePodsFromKubernetesForget(ctx, key)
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

	go func() {
		needRestart := false
		select {
		case <-vn.Done():
			logrus.WithError(vn.Err()).Infof("node exit %s", nodeID)
		case <-vn.ExitWhenLeaderChanged():
			logrus.Infof("node leader changed %s", nodeID)
			needRestart = true
		}
		vnCancel()
		brc.runtimeInfoStore.DeleteVNode(nodeID)
		if needRestart {
			brc.startVNode(nodeID, initData, t)
		}
	}()

	// leader election
	go func() {
		success := brc.vnodeLeaderElection(vnCtx, vn)
		if !success {
			// leader election failed, by node exit
			return
		}

		go vn.RenewLease(vnCtx, brc.clientID)

		go vn.Run(vnCtx)

		if err = vn.WaitReady(vnCtx, time.Minute); err != nil {
			err = errpkg.Wrap(err, "Error waiting vnode ready")
			vnCancel()
			return
		}

		brc.runtimeInfoStore.NodeRunning(nodeID)

		go utils.TimedTaskWithInterval(vnCtx, time.Second*9, func(ctx context.Context) {
			err = t.FetchHealthData(vnCtx, nodeID)
			if err != nil {
				log.G(vnCtx).WithError(err).Errorf("Failed to fetch node health info from %s", nodeID)
			}
		})

		go utils.TimedTaskWithInterval(vnCtx, time.Second*5, func(ctx context.Context) {
			err = t.QueryAllContainerStatusData(vnCtx, nodeID)
			if err != nil {
				log.G(vnCtx).WithError(err).Errorf("Failed to query containers info from %s", nodeID)
			}
		})

		// discover previous pods relate to current vnode
		listOpts := []client.ListOption{
			client.MatchingFields{"spec.nodeName": utils.FormatNodeName(nodeID, brc.env)},
			client.MatchingLabels{model.LabelKeyOfComponent: brc.vPodIdentity},
		}
		podList := &corev1.PodList{}
		err = brc.client.List(vnCtx, podList, listOpts...)
		if err != nil {
			log.G(vnCtx).WithError(err).Error("failed to list pods")
			return
		}
		brc.discoverPreviousPods(vnCtx, vn, podList)
	}()
}

func (brc *VNodeController) workloadLevel() int {
	if brc.runtimeInfoStore.AllNodeNum() == 0 {
		return 0
	}
	return brc.runtimeInfoStore.RunningNodeNum() * brc.workloadMaxLevel / (brc.runtimeInfoStore.AllNodeNum() + 1)
}

func (brc *VNodeController) delayWithWorkload(ctx context.Context) {
	if !brc.isCluster {
		// not cluster deployment, do not delay
		return
	}
	timer := time.NewTimer(100 * time.Millisecond * time.Duration(brc.workloadLevel()))
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}

func (brc *VNodeController) vnodeLeaderElection(ctx context.Context, vn *vnode.VNode) (success bool) {
	// try to create lease at start
	success = vn.CreateNodeLease(ctx, brc.clientID)
	if success {
		// create successfully
		return success
	}

	// if create failed, retry create lease when last lease deleted
	for !success {
		select {
		case <-ctx.Done():
			return false
		case <-vn.ShouldRetryLease():
			brc.delayWithWorkload(ctx)
			// retry create lease
			success = vn.CreateNodeLease(ctx, brc.clientID)
		}
	}
	return true
}

func (brc *VNodeController) shutdownVNode(nodeID string) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		return
	}
	vNode.Shutdown()
	brc.runtimeInfoStore.NodeShutdown(nodeID)
}

func (brc *VNodeController) wakeUpVNode(nodeID string) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		return
	}
	vNode.RetryLease()
}
