package vnode_controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/koupleless/virtual-kubelet/common/errdefs"
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
	"k8s.io/apimachinery/pkg/types"
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

	runtimeInfoStore *RuntimeInfoStore
}

func (brc *VNodeController) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log.G(ctx).Info("start reconcile for vpod")
	pod := &corev1.Pod{}
	err := brc.client.Get(ctx, types.NamespacedName{
		Namespace: request.Namespace,
		Name:      request.Name,
	}, pod)
	if err != nil {
		if errdefs.IsNotFound(err) {
			log.G(ctx).WithField("vpodName", pod.Name).Info("vpod has been deleted")
			return reconcile.Result{}, nil
		}
		log.G(ctx).WithField("vpodName", pod.Name).WithError(err).Info("fail to get vpod")
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: time.Second * 5,
		}, err
	}
	if pod.DeletionTimestamp != nil {
		brc.podDeleteHandler(ctx, pod)
	} else {
		brc.podAddOrUpdateHandler(ctx, pod)
	}
	return reconcile.Result{}, nil
}

func NewVNodeController(config *BuildVNodeControllerConfig) (*VNodeController, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	if len(config.Tunnels) == 0 {
		return nil, errors.New("config must have at least one tunnel provider")
	}

	if config.VPodIdentity == "" {
		return nil, errors.New("config must set vpod identity")
	}

	return &VNodeController{
		clientID:         config.ClientID,
		env:              config.Env,
		vPodIdentity:     config.VPodIdentity,
		tunnels:          config.Tunnels,
		runtimeInfoStore: NewRuntimeInfoStore(),
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

	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &handler.TypedEnqueueRequestForObject[*corev1.Pod]{}, &VPodPredicate{
		VPodIdentity: brc.vPodIdentity,
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
		},
		GenericFunc: func(ctx context.Context, e event.TypedGenericEvent[*corev1.Node], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			log.G(ctx).Info("vnode generic ", e.Object.Name)
		},
	}

	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Node{}, &nodeHandler, &VNodePredicate{})); err != nil {
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
			brc.discoverPreviousNode(ctx)
		} else {
			log.G(ctx).Error("cache sync failed")
		}
	}()

	log.G(ctx).Info("register controller ready")

	go utils.TimedTaskWithInterval(ctx, time.Second, brc.checkAndDeleteOfflineNode)

	return nil
}

func (brc *VNodeController) discoverPreviousNode(ctx context.Context) {
	componentRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
	envRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{brc.env})

	nodeList := &corev1.NodeList{}
	err := brc.client.List(ctx, nodeList, &client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*componentRequirement, *envRequirement),
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
		nodeIP := "127.0.0.1"
		nodeHostname := node.Name
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIP = addr.Address
			} else if addr.Type == corev1.NodeHostName {
				nodeHostname = addr.Address
			}
		}
		go brc.startVNode(utils.ExtractNodeIDFromNodeName(node.Name), model.NodeInfo{
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

func (brc *VNodeController) onNodeDiscovered(nodeID string, data model.NodeInfo, t tunnel.Tunnel) {
	if data.Metadata.Status == model.NodeStatusActivated {
		go brc.startVNode(nodeID, data, t)
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

	vNode.SyncAllContainerInfo(data)
}

func (brc *VNodeController) onContainerStatusChanged(nodeID string, data model.ContainerStatusData) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		return
	}
	brc.runtimeInfoStore.NodeMsgArrived(nodeID)

	vNode.SyncSingleContainerInfo(data)
}

func (brc *VNodeController) podAddOrUpdateHandler(ctx context.Context, podFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "AddOrUpdateFunc")
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
	obj, ok := vn.LoadPodFromController(key)
	isNewPod := false
	shouldEnqueue := false
	if !ok {
		vn.PodStore(key, podFromKubernetes)
		isNewPod = true
	} else {
		vn.CheckAndUpdatePod(ctx, key, obj, podFromKubernetes)
		oldPod, isPod := obj.(*corev1.Pod)
		shouldEnqueue = !isPod || podShouldEnqueue(oldPod, podFromKubernetes)
	}

	if isNewPod || shouldEnqueue {
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

func (brc *VNodeController) checkAndDeleteOfflineNode(_ context.Context) {
	offlineBase := brc.runtimeInfoStore.GetOfflineNodes(1000 * 20)
	for _, nodeID := range offlineBase {
		brc.shutdownVNode(nodeID)
	}
}

func (brc *VNodeController) startVNode(nodeID string, initData model.NodeInfo, t tunnel.Tunnel) {
	var err error
	// first apply for local lock
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "nodeID", nodeID))
	defer func() {
		if err != nil {
			log.G(ctx).WithError(err).Errorf("failed to start node %s", nodeID)
		}
		cancel()
	}()
	if initData.NetworkInfo.NodeIP == "" {
		initData.NetworkInfo.NodeIP = "127.0.0.1"
	}

	err = brc.runtimeInfoStore.PutVNodeIDNX(nodeID)
	if err != nil {
		// already exist, return
		return
	}

	vn, err := vnode.NewVNode(&vnode.BuildVNodeConfig{
		Client:       brc.client,
		KubeCache:    brc.cache,
		Tunnel:       t,
		NodeID:       nodeID,
		Env:          brc.env,
		NodeIP:       initData.NetworkInfo.NodeIP,
		NodeHostname: initData.NetworkInfo.HostName,
		NodeName:     initData.Metadata.Name,
		NodeVersion:  initData.Metadata.Version,
		CustomTaints: initData.CustomTaints,
	})
	if err != nil {
		err = errpkg.Wrap(err, "Error creating vnode")
		return
	}

	brc.runtimeInfoStore.PutVNode(nodeID, vn)
	defer brc.runtimeInfoStore.DeleteVNode(nodeID)

	go vn.Run(ctx)

	if err = vn.WaitReady(ctx, time.Minute); err != nil {
		err = errpkg.Wrap(err, "Error waiting vnode ready")
		return
	}

	t.OnNodeStart(ctx, nodeID)

	go utils.TimedTaskWithInterval(ctx, time.Second*9, func(ctx context.Context) {
		err = t.FetchHealthData(ctx, nodeID)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Failed to fetch node health info from %s", nodeID)
		}
	})

	go utils.TimedTaskWithInterval(ctx, time.Second*5, func(ctx context.Context) {
		err = t.QueryAllContainerStatusData(ctx, nodeID)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Failed to query containers info from %s", nodeID)
		}
	})

	logrus.Infof("vnode running: %s", nodeID)

	// record first msg arrived time
	brc.runtimeInfoStore.NodeMsgArrived(nodeID)

	select {
	case <-vn.Done():
		err = vn.Err()
	case <-ctx.Done():
		err = ctx.Err()
	}
}

func (brc *VNodeController) shutdownVNode(nodeID string) {
	vNode := brc.runtimeInfoStore.GetVNode(nodeID)
	if vNode == nil {
		// exited
		return
	}
	vNode.Shutdown()
}
