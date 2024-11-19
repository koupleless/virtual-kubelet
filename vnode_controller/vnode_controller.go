package vnode_controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/vnode_controller/predicates"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"time"

	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/trace"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/provider"
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
)

// VNodeController is the main controller for the virtual node
type VNodeController struct {
	clientID string // The client ID for the controller

	env string // The environment for the controller

	vPodIdentity string // The identity of the virtual pod

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

	if config.VPodIdentity == "" {
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
		vPodIdentity:     config.VPodIdentity,
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

	vpodRequirement, _ := labels.NewRequirement(model.LabelKeyOfComponent, selection.In, []string{vNodeController.vPodIdentity})

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
		utils.CheckAndFinallyCall(context.Background(), func() bool {
			if !vNodeController.tunnel.Ready() {
				return false
			}
			return true
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
			go utils.TimedTaskWithInterval(ctx, time.Millisecond*500, func(ctx context.Context) {
				outdatedVNodeNames := vNodeController.vNodeStore.GetLeaseOutdatedVNodeNames(vNodeController.clientID)
				if outdatedVNodeNames != nil && len(outdatedVNodeNames) > 0 {
					log.G(ctx).Info("check outdated vnode", outdatedVNodeNames)
				}
				for _, vNodeName := range outdatedVNodeNames {
					vNodeController.wakeUpVNode(ctx, vNodeName)
				}
			})

			// Periodically check for nodes that are not reachable and notify their leader virtual nodes.
			go utils.TimedTaskWithInterval(ctx, time.Second, func(ctx context.Context) {
				unReachableVNode := vNodeController.vNodeStore.GetUnReachableVNodes()
				if unReachableVNode != nil && len(unReachableVNode) > 0 {
					log.G(ctx).Infof("check not reachable vnode %v", unReachableVNode)
				}
				for _, vNode := range unReachableVNode {
					if vNode.IsLeader(vNodeController.clientID) {
						vNodeController.tunnel.OnNodeNotReady(ctx, vNode.GetNodeName())
					}
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
		// Initialize node IP and hostname with default values.
		nodeIP := "127.0.0.1"
		nodeHostname := node.Name
		// Iterate through the node's addresses to find the internal IP and hostname.
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIP = addr.Address
			} else if addr.Type == corev1.NodeHostName {
				nodeHostname = addr.Address
			}
		}
		// Start the virtual node with the extracted information.
		vNodeController.startVNode(model.NodeInfo{
			Metadata: model.NodeMetadata{
				Name:    node.Labels[model.LabelKeyOfVNodeName],
				Version: node.Labels[model.LabelKeyOfVNodeVersion],
			},
			NetworkInfo: model.NetworkInfo{
				NodeIP:   nodeIP,
				HostName: nodeHostname,
			},
			CustomLabels:      node.Labels,
			CustomAnnotations: node.Annotations,
			CustomTaints:      node.Spec.Taints,
			State:             model.NodeStateActivated,
		})
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
func (vNodeController *VNodeController) onBaseDiscovered(nodeName string, data model.NodeInfo) {
	if data.State == model.NodeStateActivated {
		vNodeController.startVNode(data)
	} else {
		vNodeController.shutdownVNode(nodeName)
	}
}

// onBaseStatusArrived is an event handler for when status data is received for a node.
// It updates the node's status in the virtual node.
func (vNodeController *VNodeController) onBaseStatusArrived(nodeName string, data model.NodeStatusData) {
	vNode := vNodeController.vNodeStore.GetVNode(nodeName)
	if vNode == nil {
		return
	}

	if vNode.IsLeader(vNodeController.clientID) {
		vNodeController.vNodeStore.NodeHeartbeatFromProviderArrived(nodeName)
		vNode.SyncNodeStatus(data)
	}
}

// onAllBizStatusArrived is an event handler for when status data is received for all containers in a node.
// It updates the status of all containers in the virtual node.
func (vNodeController *VNodeController) onAllBizStatusArrived(nodeName string, bizStatusDatas []model.BizStatusData) {
	vNode := vNodeController.vNodeStore.GetVNode(nodeName)
	if vNode == nil {
		return
	}

	if vNode.IsLeader(vNodeController.clientID) {
		ctx := context.Background()
		pods, _ := vNodeController.listPodFromKube(ctx, nodeName)
		bizStatusDatasWithPodKey := utils.FillPodKey(pods, bizStatusDatas)

		vNodeController.vNodeStore.NodeHeartbeatFromProviderArrived(nodeName)
		vNode.SyncAllContainerInfo(ctx, bizStatusDatasWithPodKey)
	}
}

// onSingleBizStatusArrived is an event handler for when the status of a container in a node changes.
// It updates the status of the container in the virtual node.
func (vNodeController *VNodeController) onSingleBizStatusArrived(nodeName string, bizStatusData model.BizStatusData) {
	vNode := vNodeController.vNodeStore.GetVNode(nodeName)
	if vNode == nil {
		return
	}

	if vNode.IsLeader(vNodeController.clientID) {
		ctx := context.Background()
		pods, _ := vNodeController.listPodFromKube(ctx, nodeName)
		bizStatusDatasWithPodKey := utils.FillPodKey(pods, []model.BizStatusData{bizStatusData})

		if len(bizStatusDatasWithPodKey) == 0 {
			log.G(ctx).Infof("biz container %s in k8s not found, skip sync status.", bizStatusData.Key)
			return
		}

		vNodeController.vNodeStore.NodeHeartbeatFromProviderArrived(nodeName)
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
	vn := vNodeController.vNodeStore.GetVNodeByNodeName(nodeName)
	if vn == nil {
		// node not exist, invalid add req
		return
	}

	if !vn.IsLeader(vNodeController.clientID) {
		// not leader, just return
		return
	}

	ctx, span := trace.StartSpan(ctx, "AddFunc")
	defer span.End()

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	key := utils.GetPodKey(podFromKubernetes)
	ctx = span.WithField(ctx, "key", key)

	vn.InitKnowPod(key)
	vn.SyncPodsFromKubernetesEnqueue(ctx, key)
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

	if !vNode.IsLeader(vNodeController.clientID) {
		// not leader, just return
		return
	}
	ctx, span := trace.StartSpan(ctx, "UpdateFunc")
	defer span.End()

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	key := utils.GetPodKey(newPodFromKubernetes)
	ctx = span.WithField(ctx, "key", key)
	vNode.CheckAndUpdatePodStatus(ctx, key, newPodFromKubernetes)

	if podShouldEnqueue(oldPodFromKubernetes, newPodFromKubernetes) {
		vNode.SyncPodsFromKubernetesEnqueue(ctx, key)
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
	ctx, span := trace.StartSpan(ctx, "DeleteFunc")
	defer span.End()

	key := utils.GetPodKey(podFromKubernetes)

	ctx = span.WithField(ctx, "key", key)
	vNode.DeleteKnownPod(key)
	vNode.SyncPodsFromKubernetesEnqueue(ctx, key)
	// If this pod was in the deletion queue, forget about it
	key = fmt.Sprintf("%v/%v", key, podFromKubernetes.UID)
	vNode.DeletePodsFromKubernetesForget(ctx, key)
}

// This function starts a new virtual node with the given node ID, initialization data, and tunnel.
func (vNodeController *VNodeController) startVNode(initData model.NodeInfo) {
	var err error
	// first apply for local lock
	if initData.NetworkInfo.NodeIP == "" {
		initData.NetworkInfo.NodeIP = "127.0.0.1"
	}

	nodeName := initData.Metadata.Name
	if vNodeController.vNodeStore.GetVNode(nodeName) != nil {
		// already exist, return
		return
	}

	vn, err := provider.NewVNode(&model.BuildVNodeConfig{
		Client:            vNodeController.client,
		KubeCache:         vNodeController.cache,
		Env:               vNodeController.env,
		NodeIP:            initData.NetworkInfo.NodeIP,
		NodeHostname:      initData.NetworkInfo.HostName,
		NodeName:          initData.Metadata.Name,
		NodeVersion:       initData.Metadata.Version,
		CustomTaints:      initData.CustomTaints,
		CustomLabels:      initData.CustomLabels,
		CustomAnnotations: initData.CustomAnnotations,
		WorkerNum:         vNodeController.vNodeWorkerNum,
	}, vNodeController.tunnel)
	if err != nil {
		err = errpkg.Wrap(err, "Error creating vnode")
		return
	}

	// Store the VNode in the runtime info store
	vNodeController.vNodeStore.AddVNode(nodeName, vn)

	// Create a new context with the nodeName as a value
	vnCtx := context.WithValue(context.Background(), "nodeName", nodeName)
	// Create a new context with a cancel function
	vnCtx, vnCancel := context.WithCancel(vnCtx)

	// Start a new goroutine
	go func() {
		// Initialize the needRestart variable
		needRestart := false
		// Start a select statement
		select {
		// If the VNode is done, log an error and set needRestart to true
		case <-vn.Done():
			logrus.WithError(vn.Err()).Infof("node exit %s", nodeName)
		// If the leader has changed, log a message and set needRestart to true
		case <-vn.ExitWhenLeaderChanged():
			logrus.Infof("node leader changed %s", nodeName)
			needRestart = true
		}
		// Cancel the context
		vnCancel()
		// Delete the VNode from the runtime info store
		vNodeController.vNodeStore.DeleteVNode(nodeName)
		// If needRestart is true, start a new VNode
		if needRestart {
			vNodeController.startVNode(initData)
		}
	}()

	// Start a new goroutine for leader election
	go func() {
		// Try to elect a leader
		success := vNodeController.startLeaderElection(vnCtx, vn)
		// If leader election fails, return
		if !success {
			return
		}

		// Start a new goroutine to renew the lease
		go vn.RenewLease(vnCtx, vNodeController.clientID)

		// Start a new goroutine to run the VNode
		go vn.Run(vnCtx, initData)

		// If the VNode is not ready after a minute, log an error and cancel the context
		if err = vn.WaitReady(vnCtx, time.Minute); err != nil {
			err = errpkg.Wrap(err, "Error waiting vnode ready")
			vnCancel()
			return
		}

		// Mark the node as running in the runtime info store
		vNodeController.vNodeStore.NodeHeartbeatFromProviderArrived(nodeName)

		// Start a new goroutine to fetch node health data every 10 seconds
		go utils.TimedTaskWithInterval(vnCtx, time.Second*10, func(ctx context.Context) {
			log.G(vnCtx).Info("fetch node health data for node ", nodeName)
			err = vNodeController.tunnel.FetchHealthData(vnCtx, nodeName)
			if err != nil {
				log.G(vnCtx).WithError(err).Errorf("Failed to fetch node health info from %s", nodeName)
			}
		})

		// Start a new goroutine to query all container status data every 15 seconds
		go utils.TimedTaskWithInterval(vnCtx, time.Second*15, func(ctx context.Context) {
			log.G(vnCtx).Info("query all container status data for node ", nodeName)
			err = vNodeController.tunnel.QueryAllBizStatusData(vnCtx, nodeName)
			if err != nil {
				log.G(vnCtx).WithError(err).Errorf("Failed to query containers info from %s", nodeName)
			}
		})

		// Discover previous pods related to the current VNode
		listOpts := []client.ListOption{
			client.MatchingFields{"spec.nodeName": nodeName},
			client.MatchingLabels{model.LabelKeyOfComponent: vNodeController.vPodIdentity},
		}
		podList := &corev1.PodList{}
		err = vNodeController.client.List(vnCtx, podList, listOpts...)
		if err != nil {
			log.G(vnCtx).WithError(err).Error("failed to list pods")
			return
		}
		vNodeController.discoverPreviousPods(vnCtx, vn, podList)
	}()
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

// This function attempts to elect a VNode leader by creating a lease. If the initial attempt fails, it retries after a delay.
func (vNodeController *VNodeController) startLeaderElection(ctx context.Context, vn *provider.VNode) (success bool) {
	// try to create lease at start
	success = vn.CreateNodeLease(ctx, vNodeController.clientID)
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
			vNodeController.delayWithWorkload(ctx)
			// retry create lease
			success = vn.CreateNodeLease(ctx, vNodeController.clientID)
		}
	}
	return true
}

// This function shuts down a VNode by calling its Shutdown method and updating the runtime info store.
func (vNodeController *VNodeController) shutdownVNode(nodeName string) {
	vNodeController.vNodeStore.NodeShutdown(nodeName)
}

// This function wakes up a VNode by retrying its lease if it has become outdated.
func (vNodeController *VNodeController) wakeUpVNode(ctx context.Context, nodeName string) {
	vNode := vNodeController.vNodeStore.GetVNode(nodeName)
	if vNode == nil {
		return
	}
	log.G(ctx).Info("vnode lease outdated, wake vnode up ", nodeName)
	vNode.RetryLease()
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
		return false
	}
	if !utils.PodsEqual(oldPod, newPod) {
		return true
	}
	if !deleteGraceTimeEqual(oldPod.DeletionGracePeriodSeconds, newPod.DeletionGracePeriodSeconds) {
		return true
	}
	if !oldPod.DeletionTimestamp.Equal(newPod.DeletionTimestamp) {
		return true
	}
	return false
}