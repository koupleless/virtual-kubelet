package vnode_controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/vnode_controller/predicates"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sync"
	"time"

	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/provider"
	errpkg "github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
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
)

// VNodeController is the main controller for the virtual node
type VNodeController struct {
	sync.Mutex

	clientID string // The client ID for the controller

	env string // The environment for the controller

	vPodType string // The identity of the virtual pod

	isCluster bool // Whether the controller is in a cluster

	workloadMaxLevel int // The maximum level of workload for the controller

	vNodeWorkerNum int // The number of worker nodes for the controller

	client client.Client // The client for the controller

	cache cache.Cache // The cache for the controller

	ready chan struct{} // The channel for the controller to be ready

	tunnel tunnel.Tunnel

	vNodeStore *provider.VNodeStore // The runtime info store for the controller
}

// Reconcile is the main reconcile function for the controller
func (vNodeController *VNodeController) Reconcile(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
	// do nothing here
	return reconcile.Result{}, nil
}

// NewVNodeController creates a new VNodeController
func NewVNodeController(config *model.BuildVNodeControllerConfig, tunnel tunnel.Tunnel) (*VNodeController, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	if config.VPodType == "" {
		return nil, errors.New("config must set vpod identity")
	}

	if config.IsCluster && config.WorkloadMaxLevel == 0 {
		config.WorkloadMaxLevel = 3
	}

	if config.VNodeWorkerNum == 0 {
		config.VNodeWorkerNum = 1
	}

	return &VNodeController{
		clientID:         config.ClientID,
		env:              config.Env,
		client:           config.KubeClient,
		cache:            config.KubeCache,
		vPodType:         config.VPodType,
		isCluster:        config.IsCluster,
		workloadMaxLevel: config.WorkloadMaxLevel,
		vNodeWorkerNum:   config.VNodeWorkerNum,
		vNodeStore:       provider.NewVNodeStore(),
		ready:            make(chan struct{}),
		tunnel:           tunnel,
	}, nil
}

// SetupWithManager sets up the controller with the manager
func (vNodeController *VNodeController) SetupWithManager(ctx context.Context, mgr manager.Manager) (err error) {
	// init  tunnel
	vNodeController.tunnel.RegisterCallback(vNodeController.onBaseDiscovered, vNodeController.onBaseStatusArrived, vNodeController.onAllBizStatusArrived, vNodeController.onSingleBizStatusArrived)

	vNodeController.client = mgr.GetClient()
	vNodeController.cache = mgr.GetCache()

	log.G(ctx).Info("Setting up register controller")

	if err = mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, "spec.nodeName", func(rawObj client.Object) []string {
		pod := rawObj.(*corev1.Pod)
		return []string{pod.Spec.NodeName}
	}); err != nil {
		return err
	}

	c, err := controller.New("vnode-controller", mgr, controller.Options{
		Reconciler: vNodeController,
	})
	if err != nil {
		log.G(ctx).Error(err, "unable to set up vnode controller")
		return err
	}

	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{vNodeController.vPodType})

	podHandler := handler.TypedFuncs[*corev1.Pod, reconcile.Request]{
		CreateFunc: func(ctx context.Context, e event.TypedCreateEvent[*corev1.Pod], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			<-vNodeController.ready
			vNodeController.podAddHandler(ctx, e.Object)
		},
		UpdateFunc: func(ctx context.Context, e event.TypedUpdateEvent[*corev1.Pod], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			<-vNodeController.ready
			vNodeController.podUpdateHandler(ctx, e.ObjectOld, e.ObjectNew)
		},
		DeleteFunc: func(ctx context.Context, e event.TypedDeleteEvent[*corev1.Pod], w workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			<-vNodeController.ready
			vNodeController.podDeleteHandler(ctx, e.Object)
		},
	}

	if err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}, &podHandler, &predicates.VPodPredicate{
		VPodLabelSelector: labels.NewSelector().Add(*vpodRequirement),
	})); err != nil {
		log.G(ctx).WithError(err).Error("unable to watch Pods")
		return err
	}

	go func() {
		// wait for all tunnel to be ready
		utils.CheckAndFinallyCall(context.Background(), func(ctx context.Context) (bool, error) {
			if !vNodeController.tunnel.Ready() {
				return false, nil
			}
			return true, nil
		}, time.Minute, time.Second, func() {
			log.G(ctx).Infof("tunnel %v are ready", vNodeController.tunnel)
		}, func() {
			log.G(ctx).Errorf("waiting for tunnel %v to be ready timeout", vNodeController.tunnel)
		})

		synced := vNodeController.cache.WaitForCacheSync(ctx)
		if synced {
			// This section is responsible for restarting the virtual kubelet for previous nodes.
			componentRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{model.ComponentVNode})
			envRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{vNodeController.env})

			nodeList := &corev1.NodeList{}
			err = vNodeController.client.List(ctx, nodeList, &client.ListOptions{
				LabelSelector: labels.NewSelector().Add(*componentRequirement, *envRequirement),
			})
			if err != nil {
				log.G(ctx).WithError(err).Error("failed to list nodes")
				return
			}

			// Discover and process previous nodes to ensure they are properly registered.
			vNodeController.discoverPreviousNodes(nodeList)

			// Periodically check for outdated virtual nodes and wake them up if necessary.
			go utils.TimedTaskWithInterval(ctx, 5*time.Second, func(ctx context.Context) {
				outdatedVNodeNames := vNodeController.vNodeStore.GetLeaseOutdatedVNodeNames(vNodeController.clientID)
				if outdatedVNodeNames != nil && len(outdatedVNodeNames) > 0 {
					nodeNames := make([]string, 0, len(outdatedVNodeNames))
					for _, vNodeName := range outdatedVNodeNames {
						nodeNames = append(nodeNames, vNodeName)
					}
					log.G(ctx).Info("check outdated vnode", nodeNames)
				}
			})

			// Signal that the controller is ready.
			close(vNodeController.ready)
		} else {
			log.G(ctx).Error("cache sync failed")
		}
	}()

	log.G(ctx).Info("register controller ready")

	return nil
}

// This function discovers and processes previous nodes to ensure they are properly registered.
func (vNodeController *VNodeController) discoverPreviousNodes(nodeList *corev1.NodeList) {
	// Iterate through the list of nodes to process each node.
	for _, node := range nodeList.Items {
		// Start the virtual node with the extracted information.
		vNodeController.startVNode(utils.ConvertNodeToNodeInfo(&node))
	}
}

// This function discovers and processes previous pods to ensure they are properly registered.
func (vNodeController *VNodeController) discoverPreviousPods(ctx context.Context, vNode *provider.VNode, podList *corev1.PodList) {
	// Iterate through the list of pods to process each pod.
	for _, pod := range podList.Items {
		// Generate a unique key for the pod.
		key := utils.GetPodKey(&pod)
		// Sync the pods from Kubernetes to the virtual node.
		vNode.SyncPodsFromKubernetesEnqueue(ctx, key)
	}
}

// The following functions are event handlers for various node and pod events.
// They are used to manage the state of the virtual nodes and synchronize the node and pod information.

// onBaseDiscovered is an event handler for when a new node is discovered.
// It starts a virtual node if the node's status is activated, otherwise it shuts down the virtual node.
func (vNodeController *VNodeController) onBaseDiscovered(data model.NodeInfo) {
	if data.State == model.NodeStateActivated {
		vNodeController.startVNode(data)
	} else {
		// TODO: update node status
	}
	vNodeController.vNodeStore.UpdateNodeStateOnProviderArrived(data.Metadata.Name, data.State)
}

// onBaseStatusArrived is an event handler for when status data is received for a node.
// It updates the node's status in the virtual node.
func (vNodeController *VNodeController) onBaseStatusArrived(nodeName string, data model.NodeStatusData) {
	vNode := vNodeController.vNodeStore.GetVNode(nodeName)

	// if not exist then return
	if vNode == nil {
		return
	}

	if vNode.IsLeader(vNodeController.clientID) {
		vNode.SyncNodeStatus(data)
	}
}

// onAllBizStatusArrived is an event handler for when status data is received for all containers in a node.
// It updates the status of all containers in the virtual node.
func (vNodeController *VNodeController) onAllBizStatusArrived(nodeName string, bizStatusDatas []model.BizStatusData) {
	vNode := vNodeController.vNodeStore.GetVNode(nodeName)

	// if not exist then return
	if vNode == nil {
		return
	}

	if vNode.IsLeader(vNodeController.clientID) {
		ctx := context.Background()
		pods, _ := vNodeController.listPodFromKube(ctx, nodeName)
		bizStatusDatasWithPodKey, _ := utils.FillPodKey(pods, bizStatusDatas)

		vNode.SyncAllContainerInfo(ctx, bizStatusDatasWithPodKey)
	}
}

// onSingleBizStatusArrived is an event handler for when the status of a container in a node changes.
// It updates the status of the container in the virtual node.
func (vNodeController *VNodeController) onSingleBizStatusArrived(nodeName string, bizStatusData model.BizStatusData) {
	vNode := vNodeController.vNodeStore.GetVNode(nodeName)

	// if not exist then return
	if vNode == nil {
		return
	}

	if vNode.IsLeader(vNodeController.clientID) {
		ctx := context.Background()
		pods, _ := vNodeController.listPodFromKube(ctx, nodeName)
		bizStatusDatasWithPodKey, _ := utils.FillPodKey(pods, []model.BizStatusData{bizStatusData})

		if len(bizStatusDatasWithPodKey) == 0 {
			return
		}

		vNode.SyncOneContainerInfo(context.TODO(), bizStatusDatasWithPodKey[0])
	}
}

// podAddHandler is an event handler for when a new pod is created.
// It syncs the pod from Kubernetes to the virtual node.
func (vNodeController *VNodeController) podAddHandler(ctx context.Context, podFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	nodeName := podFromKubernetes.Spec.NodeName
	// check node name in local storage
	vNode := vNodeController.vNodeStore.GetVNodeByNodeName(nodeName)
	if vNode == nil {
		// node not exist, invalid add req
		return
	}

	podKey := utils.GetPodKey(podFromKubernetes)
	if _, has := vNode.GetKnownPod(podKey); !has {
		vNode.AddKnowPod(podFromKubernetes)
	}

	log.G(ctx).Infof("try to add pod %s with handler in vnode: %s", podKey, vNode.GetNodeName())
	if !vNode.IsLeader(vNodeController.clientID) {
		log.G(ctx).Infof("can not add pod %s because is not the leader of vnode: %s", podKey, vNode.GetNodeName())
		return
	}

	if !vNodeController.isValidStatus(ctx, vNode) {
		log.G(ctx).Warnf("can not add pod %s because vnode %s is invalid: ", podKey, vNode.GetNodeName())
		return
	}

	log.G(ctx).Infof("start to add pod %s with handler in vnode: %s", podKey, vNode.GetNodeName())

	ctx, span := trace.StartSpan(ctx, "AddFunc")
	defer span.End()

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	ctx = span.WithField(ctx, "key", podKey)

	vNode.SyncPodsFromKubernetesEnqueue(ctx, podKey)
}

func (vNodeController *VNodeController) isValidStatus(ctx context.Context, vNode *provider.VNode) bool {
	if !vNode.IsReady() {
		log.G(ctx).Warnf("vnode %s is not ready: ", vNode.GetNodeName())
		return false
	}

	if vNode.Liveness.IsDead() {
		log.G(ctx).Warnf("check and shutdown dead vnode: %s", vNode.GetNodeName())
		vNodeController.shutdownVNode(vNode.GetNodeName())
		return false
	}

	if !vNode.Liveness.IsReachable() {
		log.G(ctx).Warnf("node %s is not reachable", vNode.GetNodeName())
		return false
	}

	return true
}

// This function handles pod updates by checking if the pod is new or if its status has changed.
func (vNodeController *VNodeController) podUpdateHandler(ctx context.Context, oldPodFromKubernetes, newPodFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	nodeName := newPodFromKubernetes.Spec.NodeName
	// check node name in local storage
	vNode := vNodeController.vNodeStore.GetVNodeByNodeName(nodeName)
	if vNode == nil {
		// node not exist, invalid add req
		return
	}

	podKey := utils.GetPodKey(newPodFromKubernetes)
	if _, has := vNode.GetKnownPod(podKey); !has {
		vNode.AddKnowPod(newPodFromKubernetes)
	}

	log.G(ctx).Infof("try to update pod %s with handler in vnode: %s", podKey, vNode.GetNodeName())
	if !vNode.IsLeader(vNodeController.clientID) {
		log.G(ctx).Infof("can not update pod %s because is not the leader of vnode: %s", podKey, vNode.GetNodeName())
		return
	}

	if !vNodeController.isValidStatus(ctx, vNode) {
		log.G(ctx).Warnf("can not update pod %s because vnode %s is invalid: ", podKey, vNode.GetNodeName())
		return
	}

	ctx, span := trace.StartSpan(ctx, "UpdateFunc")
	defer span.End()

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	ctx = span.WithField(ctx, "key", podKey)
	vNode.CheckAndUpdatePodStatus(ctx, podKey, newPodFromKubernetes)

	if podShouldEnqueue(oldPodFromKubernetes, newPodFromKubernetes) {
		log.G(ctx).Infof("start to update pod %s(old) -> %s(new) with handler in node: %s", utils.GetPodKey(oldPodFromKubernetes), utils.GetPodKey(newPodFromKubernetes), vNode.GetNodeName())
		vNode.SyncPodsFromKubernetesEnqueue(ctx, podKey)
	}
}

// This function handles pod deletions by removing the pod from the virtual node's state.
func (vNodeController *VNodeController) podDeleteHandler(ctx context.Context, podFromKubernetes *corev1.Pod) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	nodeName := podFromKubernetes.Spec.NodeName
	// check node name in local storage
	vNode := vNodeController.vNodeStore.GetVNodeByNodeName(nodeName)
	if vNode == nil {
		// node not exist, invalid add req
		return
	}

	if !vNode.IsLeader(vNodeController.clientID) {
		// not leader, just return
		return
	}

	podKey := utils.GetPodKey(podFromKubernetes)
	if !vNodeController.isValidStatus(ctx, vNode) {
		log.G(ctx).Warnf("can not delete pod %s because vnode %s is invalid: ", podKey, vNode.GetNodeName())
		return
	}

	log.G(ctx).Infof("start to delete pod %s with handler in node: %s", utils.GetPodKey(podFromKubernetes), vNode.GetNodeName())

	ctx, span := trace.StartSpan(ctx, "DeleteFunc")
	defer span.End()

	ctx = span.WithField(ctx, "key", podKey)
	vNode.DeleteKnownPod(podKey)
	vNode.SyncPodsFromKubernetesEnqueue(ctx, podKey)
	// If this pod was in the deletion queue, forget about it
	key := fmt.Sprintf("%v/%v", podKey, podFromKubernetes.UID)
	vNode.DeletePodsFromKubernetesForget(ctx, key)
}

// This function starts a new virtual node with the given node ID, initialization data, and tunnel.
func (vNodeController *VNodeController) startVNode(initData model.NodeInfo) {
	vNodeController.Lock()
	defer vNodeController.Unlock()

	nodeName := initData.Metadata.Name
	vNode := vNodeController.vNodeStore.GetVNode(nodeName)
	if vNode != nil {
		return
	}

	vNodeController.createAndRunVNode(initData)
}

func (vNodeController *VNodeController) createAndRunVNode(initData model.NodeInfo) {
	nodeName := initData.Metadata.Name
	vnCtx, vnCtxCancel := context.WithCancel(context.WithValue(context.Background(), "nodeName", nodeName))

	vNode, err := vNodeController.createVNode(vnCtx, initData)
	if err != nil {
		err = errpkg.Wrap(err, "Error creating vnode")
		vnCtxCancel()
		return
	}

	vNodeController.runVNode(vnCtx, vNode, initData)

	go func() {
		select {
		case <-vNode.Done():
			log.G(vnCtx).Infof("vnode done: %s, try to delete node", vNode.GetNodeName())
			vNodeController.deleteVNode(vnCtx, vNode)
			log.G(vnCtx).Infof("vnode done: %s, deleted node", vNode.GetNodeName())
			vnCtxCancel()
		}
	}()
}

func (vNodeController *VNodeController) deleteVNode(vnCtx context.Context, vNode *provider.VNode) {
	log.G(vnCtx).Infof("start to remove vnode %s because vnode exited", vNode.GetNodeName())

	vNodeController.vNodeStore.DeleteVNode(vNode.GetNodeName())

	err := vNode.Remove(vnCtx)
	if err != nil {
		log.G(vnCtx).WithError(err).Errorf("failed to remove node %s", vNode.GetNodeName())
	}

	log.G(vnCtx).Infof("remove node %s success", vNode.GetNodeName())
}

func (vNodeController *VNodeController) createVNode(vnCtx context.Context, initData model.NodeInfo) (kn *provider.VNode, err error) {
	nodeName := initData.Metadata.Name
	log.G(vnCtx).Infof("create vnode %s", nodeName)

	if initData.NetworkInfo.NodeIP == "" {
		initData.NetworkInfo.NodeIP = "127.0.0.1"
	}

	var vNode *provider.VNode
	vNode, err = provider.NewVNode(&model.BuildVNodeConfig{
		Client:            vNodeController.client,
		KubeCache:         vNodeController.cache,
		NodeIP:            initData.NetworkInfo.NodeIP,
		NodeHostname:      initData.NetworkInfo.HostName,
		NodeName:          initData.Metadata.Name,
		BaseName:          initData.Metadata.BaseName,
		NodeVersion:       initData.Metadata.Version,
		VPodType:          vNodeController.vPodType,
		ClusterName:       initData.Metadata.ClusterName,
		Env:               vNodeController.env,
		CustomTaints:      initData.CustomTaints,
		CustomLabels:      initData.CustomLabels,
		CustomAnnotations: initData.CustomAnnotations,
		WorkerNum:         vNodeController.vNodeWorkerNum,
	}, vNodeController.tunnel)
	if err != nil {
		err = errpkg.Wrap(err, "Error new vnode: "+nodeName)
		return nil, err
	}

	err = vNodeController.vNodeStore.AddVNode(nodeName, vNode)
	if err != nil {
		err = errpkg.Wrap(err, "Error addVNode vnode: "+nodeName)
		return nil, err
	}

	log.G(vnCtx).Infof("created vnode %s success", vNode.GetNodeName())
	return vNode, err
}

func (vNodeController *VNodeController) runVNode(vnCtx context.Context, vNode *provider.VNode, initData model.NodeInfo) {
	log.G(vnCtx).Infof("start to run vnode %s", vNode.GetNodeName())
	go vNodeController.startLeaderElection(vnCtx, vNode)

	go func() {
		for {
			select {
			case <-vNode.WhenLeaderAcquiredByMe:
				vNodeController.takeOverVNode(vnCtx, vNode, initData)
			case <-vnCtx.Done():
				return
			}
		}
	}()
}

func (vNodeController *VNodeController) startLeaderElection(vnCtx context.Context, vNode *provider.VNode) {
	utils.TimedTaskWithInterval(vnCtx, time.Second*model.NodeLeaseUpdatePeriodSeconds, func(vnCtx context.Context) {
		vNodeController.createOrRetryUpdateLease(vnCtx, vNode)
	})
}

func (vNodeController *VNodeController) createOrRetryUpdateLease(vnCtx context.Context, vNode *provider.VNode) {
	log.G(vnCtx).Infof("try to acquire node lease for %s by %s", vNode.GetNodeName(), vNodeController.clientID)
	for i := 0; i < model.NodeLeaseMaxRetryTimes; i++ {
		time.Sleep(time.Millisecond * 200) // TODO: add random sleep time for reduce the client rate
		lease := &coordinationv1.Lease{}
		err := vNodeController.client.Get(vnCtx, types.NamespacedName{
			Name:      vNode.GetNodeName(),
			Namespace: corev1.NamespaceNodeLease,
		}, lease)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// If not found, try to create a new lease
				lease = vNode.NewLease(vNodeController.clientID)

				// Attempt to create the lease
				err = vNodeController.client.Create(vnCtx, lease)
				// If the context is canceled or deadline exceeded, return false
				if err != nil {
					// Log the error if there's a problem creating the lease
					log.G(vnCtx).WithError(err).Errorf("node lease %s creating error", vNode.GetNodeName())
				}
				log.G(vnCtx).Infof("node lease %s created: %s", vNode.GetNodeName(), lease.Spec.RenewTime)
				continue
			}

			log.G(vnCtx).WithError(err).WithField("retries", i).Error("failed to get node lease when updating node lease")
			continue
		}

		isLeaderBefore := vNode.IsLeader(vNodeController.clientID)
		vNode.SetLease(lease)
		isLeaderNow := vNode.IsLeader(vNodeController.clientID)
		// If the holder identity is not the current client id, the leader has changed
		if isLeaderBefore && !isLeaderNow {
			log.G(vnCtx).Infof("node lease %s acquired by %s", vNode.GetNodeName(), *vNode.GetLease().Spec.HolderIdentity)
			vNode.LeaderAcquiredByOthers()
			return
		} else if isLeaderNow && !vNode.TakeOvered {
			log.G(vnCtx).Infof("node lease %s acquired by %s", vNode.GetNodeName(), vNodeController.clientID)
			vNode.LeaderAcquiredByMe()
			log.G(vnCtx).Infof("node %s inited after leader acquired", vNode.GetNodeName())
		}

		newLease := lease.DeepCopy()
		newLease.Spec.HolderIdentity = &vNodeController.clientID
		newLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}

		err = vNodeController.client.Patch(vnCtx, newLease, client.MergeFrom(lease))
		if err == nil {
			log.G(vnCtx).WithField("retries", i).Infof("Successfully updated lease for %s", vNode.GetNodeName())
			vNode.SetLease(lease)
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.G(vnCtx).WithError(err).Errorf("failed to update node lease for %s in retry %d/%d, and stop retry.", vNode.GetNodeName(), i, model.NodeLeaseMaxRetryTimes)
			return
		}
		// OptimisticLockError requires getting the newer version of lease to proceed.
		if apierrors.IsConflict(err) {
			log.G(vnCtx).WithError(err).Errorf("failed to update node lease for %s in retry %d/%d", vNode.GetNodeName(), i, model.NodeLeaseMaxRetryTimes)
			continue
		}
	}
}

func (vNodeController *VNodeController) takeOverVNode(vnCtx context.Context, vNode *provider.VNode, initData model.NodeInfo) {
	log.G(context.Background()).Infof("start to take over vnode %s", vNode.GetNodeName())
	vNode.TakeOvered = true
	defer func() {
		vNode.TakeOvered = false
	}()

	takeOverVnCtx, takeOverCancel := context.WithCancel(context.WithValue(vnCtx, "nodeName", vNode.GetNodeName()))
	defer takeOverCancel()

	var err error

	err = vNode.Run(vnCtx, takeOverVnCtx, initData)
	if err != nil {
		log.G(takeOverVnCtx).WithError(err).Errorf("failed to run vnode and release: %s", vNode.GetNodeName())
		takeOverCancel()
		return
	}

	if err = vNode.WaitReady(takeOverVnCtx, time.Minute); err != nil {
		log.G(takeOverVnCtx).WithError(err).Errorf("vnode is not ready: %s", vNode.GetNodeName())
		takeOverCancel()
		return
	}

	vNodeController.connectWithInterval(takeOverVnCtx, vNode)

	log.G(takeOverVnCtx).Infof("take over vnode %s completed", vNode.GetNodeName())

	select {
	case <-vNode.WhenLeaderAcquiredByOthers:
		log.G(takeOverVnCtx).Infof("release vnode %s because leader is acquired by others", vNode.GetNodeName())
		takeOverCancel()
		return
	case <-vnCtx.Done():
		log.G(takeOverVnCtx).Infof("release vnode %s because vnCtx is done", vNode.GetNodeName())
		takeOverCancel()
		return
	}
}

func (vNodeController *VNodeController) connectWithInterval(takeOverVnCtx context.Context, vNode *provider.VNode) {
	nodeName := vNode.GetNodeName()

	var err error

	// Start a new goroutine to fetch node health data every NodeToFetchHeartBeatInterval seconds
	go utils.TimedTaskWithInterval(takeOverVnCtx, time.Second*model.NodeToFetchHeartBeatInterval, func(ctx context.Context) {
		log.G(takeOverVnCtx).Info("fetch node health data for node ", nodeName)
		err = vNodeController.tunnel.FetchHealthData(nodeName)
		if err != nil {
			log.G(takeOverVnCtx).WithError(err).Errorf("Failed to fetch node health info from %s", nodeName)
		}
	})

	// Start a new goroutine to query all container status data every NodeToFetchAllBizStatusInterval seconds
	go utils.TimedTaskWithInterval(takeOverVnCtx, time.Second*model.NodeToFetchAllBizStatusInterval, func(context.Context) {
		log.G(takeOverVnCtx).Info("query all container status data for node ", nodeName)
		err = vNodeController.tunnel.QueryAllBizStatusData(nodeName)
		if err != nil {
			log.G(takeOverVnCtx).WithError(err).Errorf("Failed to query containers info from %s", nodeName)
		}
	})

	go utils.TimedTaskWithInterval(takeOverVnCtx, model.NodeToCheckUnreachableAndDeadStatusInterval*time.Second, func(takeOverVnCtx context.Context) {
		if vNode.Liveness.IsDead() {
			log.G(takeOverVnCtx).Infof("check and shutdown dead vnode: %s", nodeName)
			vNodeController.shutdownVNode(vNode.GetNodeName())
			return
		}

		if !vNode.Liveness.IsReachable() {
			log.G(takeOverVnCtx).Warnf("node %s is not reachable in interval checking", nodeName)
		}
	})
}

// This function calculates the workload level based on the number of running nodes and the total number of nodes.
func (vNodeController *VNodeController) workloadLevel() int {
	if vNodeController.vNodeStore.AllNodeNum() == 0 {
		return 0
	}
	return vNodeController.vNodeStore.RunningNodeNum() * vNodeController.workloadMaxLevel / (vNodeController.vNodeStore.AllNodeNum() + 1)
}

// This function introduces a delay based on the workload level in a cluster deployment.
func (vNodeController *VNodeController) delayWithWorkload(ctx context.Context) {
	if !vNodeController.isCluster {
		// not cluster deployment, do not delay
		return
	}
	timer := time.NewTimer(100 * time.Millisecond * time.Duration(vNodeController.workloadLevel()))
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}

// This function shuts down a VNode by calling its Shutdown method and updating the runtime info store.
func (vNodeController *VNodeController) shutdownVNode(nodeName string) {
	vNodeController.vNodeStore.NodeShutdown(nodeName)
}

// getPodFromKube loads a pod from the node's pod controller
func (vNodeController *VNodeController) getPodFromKube(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	err := vNodeController.cache.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, pod, &client.GetOptions{})
	return pod, err
}

// ListPodFromKube list all pods for this node from the kubernetes
func (vNodeController *VNodeController) listPodFromKube(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := vNodeController.cache.List(ctx, podList, &client.ListOptions{FieldSelector: fields.SelectorFromSet(
		fields.Set{"spec.nodeName": nodeName})})
	return podList.Items, err
}

// deleteGraceTimeEqual: This function checks if two int64 pointers are equal.
// Parameters:
// - old: The old int64 pointer.
// - new: The new int64 pointer.
// Returns: A boolean value indicating if the two int64 pointers are equal.
func deleteGraceTimeEqual(old, new *int64) bool {
	if old == nil && new == nil {
		return true
	}
	if old != nil && new != nil {
		return *old == *new
	}
	return false
}

// podShouldEnqueue: This function checks if two pods are equal according to the podsEqual function and the DeletionTimeStamp.
// Parameters:
// - oldPod: The old pod.
// - newPod: The new pod.
// Returns: A boolean value indicating if the two pods are equal.
func podShouldEnqueue(oldPod, newPod *corev1.Pod) bool {
	if oldPod == nil || newPod == nil {
		log.L.Warnf("pod will not update because %s(old) or %s(new) is nil", utils.GetPodKey(oldPod), utils.GetPodKey(newPod))
		return false
	}
	if !utils.PodsEqual(oldPod, newPod) {
		log.L.Infof("pod will update because %s(old) -> %s(new) info is updated (new)", utils.GetPodKey(oldPod), utils.GetPodKey(newPod))
		return true
	}
	if podShouldEnqueueForDelete(oldPod, newPod) {
		log.L.Infof("pod will update for delete %s", utils.GetPodKey(oldPod))
		return true
	}

	if podShouldEnqueueForAdd(oldPod, newPod) {
		log.L.Infof("pod will update for add %s", utils.GetPodKey(oldPod))
		return true
	}

	log.L.Infof("pod %s(old) -> %s(new) will not execute", utils.GetPodKey(oldPod), utils.GetPodKey(newPod))
	return false
}

func podShouldEnqueueForDelete(oldPod, newPod *corev1.Pod) bool {
	if !deleteGraceTimeEqual(oldPod.DeletionGracePeriodSeconds, newPod.DeletionGracePeriodSeconds) {
		return true
	}
	if !oldPod.DeletionTimestamp.Equal(newPod.DeletionTimestamp) {
		return true
	}
	return false
}

func podShouldEnqueueForAdd(oldPod, newPod *corev1.Pod) bool {
	if oldPod.Spec.NodeName == "" && newPod.Spec.NodeName != "" {
		return true
	}
	return false
}
