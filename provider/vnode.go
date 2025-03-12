package provider

import (
	"context"
	"fmt"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/node"
	nodeutil2 "github.com/koupleless/virtual-kubelet/virtual_kubelet/node/nodeutil"
	"k8s.io/apimachinery/pkg/api/resource"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"time"

	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// VNode is the main struct for a virtual node
type VNode struct {
	name      string        // Unique identifier of the node
	env       string        // Environment of the node
	client    client.Client // Kubernetes client
	kubeCache cache.Cache   // Kubernetes cache

	nodeProvider *VNodeProvider  // Node provider for the virtual node
	podProvider  *VPodProvider   // Pod provider for the virtual node
	vpodType     string          // VPod type for the virtual node
	node         *nodeutil2.Node // Node instance for the virtual node
	tunnel       tunnel.Tunnel

	exit                       chan struct{} // Channel for signaling the node to exit
	ready                      chan struct{} // Channel for signaling the node is ready
	WhenLeaderAcquiredByOthers chan struct{} // Channel for signaling the leader has changed
	WhenLeaderAcquiredByMe     chan struct{}
	done                       chan struct{} // Channel for signaling the node has exited

	lease    *coordinationv1.Lease // Latest lease of the node
	Liveness Liveness              // Liveness of the node from provider

	TakeOvered bool  // take overed by current vnodeController
	err        error // Error that caused the node to exit
}

func (vNode *VNode) GetNodeName() string {
	return vNode.name
}

func (vNode *VNode) GetLease() *coordinationv1.Lease {
	return vNode.lease
}

func (vNode *VNode) SetLease(lease *coordinationv1.Lease) {
	vNode.lease = lease
}

func (vNode *VNode) Remove(vnCtx context.Context) (err error) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: vNode.name,
		},
	}

	err = vNode.client.Delete(vnCtx, node)
	if err != nil && !apierrors.IsNotFound(err) {
		log.G(vnCtx).WithError(err).Errorf("failed to remove node %s in k8s", vNode.GetNodeName())
		return err
	}

	err = vNode.client.Get(vnCtx, types.NamespacedName{
		Name: vNode.name}, node)
	if err != nil && !apierrors.IsNotFound(err) {
		log.G(vnCtx).WithError(err).Errorf("failed to get node %s in k8s when removing", vNode.GetNodeName())
		return err
	}

	err = vNode.client.Delete(vnCtx, vNode.lease)
	if err != nil && !apierrors.IsNotFound(err) {
		log.G(vnCtx).WithError(err).Errorf("failed to remove node lease for %s in k8s", vNode.GetNodeName())
		return err
	}
	return err
}

func (vNode *VNode) Run(vnCtx context.Context, takeOverVnCtx context.Context, initData model.NodeInfo) (err error) {
	defer func() {
		vNode.err = err
	}()

	vNode.resetActivationStatus()

	go func() {
		err = vNode.node.Run(takeOverVnCtx)
		if err != nil {
			log.G(takeOverVnCtx).WithError(err).Errorf("failed to run node: %s", vNode.GetNodeName())
		}
	}()

	err = vNode.node.WaitReady(takeOverVnCtx, time.Minute)
	if err != nil {
		log.G(takeOverVnCtx).WithError(err).Errorf("Error waiting node ready: %s", vNode.GetNodeName())
		return err
	}

	err = utils.CheckAndFinallyCall(takeOverVnCtx, vNode.checkNodeExistsInClient, time.Minute, time.Millisecond*200, func() {}, func() {})
	if err != nil {
		log.G(takeOverVnCtx).WithError(err).Errorf("Error checking node exists: %s", vNode.GetNodeName())
		return err
	}

	log.G(takeOverVnCtx).Infof("Node exists: %s", vNode.GetNodeName())
	err = vNode.tunnel.RegisterNode(initData)
	if err != nil {
		log.G(takeOverVnCtx).WithError(err).Errorf("Error register node: %s in tunnel: %s", vNode.GetNodeName(), vNode.tunnel.Key())
		return err
	}

	go func() {
		select {
		case <-vNode.Exit():
			vNode.cleanUp()
			log.G(vnCtx).Infof("clear resources vnode %s completed, to state: done", vNode.GetNodeName())
			vNode.ToDone()
		case <-takeOverVnCtx.Done():
			vNode.cleanUp()
		}
	}()

	vNode.activate()

	vNode.discoveryPreviousPods(takeOverVnCtx)

	return nil
}

func (vNode *VNode) cleanUp() {
	vNode.resetActivationStatus()
	vNode.tunnel.UnRegisterNode(vNode.name)
}

func (vNode *VNode) checkNodeExistsInClient(vnCtx context.Context) (bool, error) {
	vnode := &corev1.Node{}
	err := vNode.kubeCache.Get(vnCtx, types.NamespacedName{
		Name: vNode.name,
	}, vnode)
	return err == nil, err
}

func (vNode *VNode) discoveryPreviousPods(ctx context.Context) {
	log.G(ctx).Infof("discovery previous pods for %s", vNode.name)
	// Discover previous pods related to the current VNode
	listOpts := []client.ListOption{
		client.MatchingFields{"spec.nodeName": vNode.name},
		client.MatchingLabels{model.LabelKeyOfComponent: vNode.vpodType},
	}
	podList := &corev1.PodList{}
	err := vNode.client.List(ctx, podList, listOpts...)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to list pods")
		return
	}

	// Iterate through the list of pods to process each pod.
	for _, pod := range podList.Items {
		// Generate a unique key for the pod.
		key := utils.GetPodKey(&pod)
		// Sync the pods from Kubernetes to the virtual node.
		vNode.SyncPodsFromKubernetesEnqueue(ctx, key)
	}
}

// WaitReady waits for the node to be ready
func (vNode *VNode) WaitReady(ctx context.Context, timeout time.Duration) error {
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	}

	select {
	case <-vNode.ready:
		return nil
	case <-vNode.done:
		return fmt.Errorf("vnode exited before ready: %w", vNode.err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (vNode *VNode) activate() {
	close(vNode.ready)
}

func (vNode *VNode) IsReady() bool {
	select {
	case <-vNode.ready:
		return true
	default:
		return false
	}
}

func (vNode *VNode) resetActivationStatus() {
	vNode.ready = make(chan struct{})
}

// NewLease creates a new lease for the node
func (vNode *VNode) NewLease(holderIdentity string) *coordinationv1.Lease {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vNode.name,
			Namespace: corev1.NamespaceNodeLease,
			Labels: map[string]string{
				model.LabelKeyOfEnv:       vNode.env,
				model.LabelKeyOfComponent: model.ComponentVNodeLease,
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To(holderIdentity),
			LeaseDurationSeconds: ptr.To[int32](model.NodeLeaseDurationSeconds),
			RenewTime:            &metav1.MicroTime{Time: time.Now()},
		},
	}

	return lease
}

// SyncNodeStatus syncs the status of the node
func (vNode *VNode) SyncNodeStatus(data model.NodeStatusData) {
	if vNode.nodeProvider != nil {
		vNode.nodeProvider.Notify(data)
	}
}

// SyncBatchBizStatusToKube syncs the status of all containers
func (vNode *VNode) SyncBatchBizStatusToKube(ctx context.Context, toUpdateInKube []model.BizStatusData, toDeleteInProvider []model.BizStatusData) {
	if vNode.podProvider != nil {
		vNode.podProvider.SyncAllBizStatusToKube(ctx, toUpdateInKube)
		vNode.syncNotExistBizPodToProvider(ctx, toDeleteInProvider)
	}
}

// SyncOneNodeBizStatusToKube syncs the status of a single container
func (vNode *VNode) SyncOneNodeBizStatusToKube(ctx context.Context, toUpdateInKube []model.BizStatusData, toDeleteInProvider []model.BizStatusData) {
	if vNode.podProvider != nil {
		for _, bizStatus := range toUpdateInKube {
			vNode.podProvider.SyncBizStatusToKube(ctx, bizStatus)
		}
		vNode.syncNotExistBizPodToProvider(ctx, toDeleteInProvider)
	}
}

func (vNode *VNode) syncNotExistBizPodToProvider(ctx context.Context, toDeleteInProvider []model.BizStatusData) {
	for _, bizStatus := range toDeleteInProvider {
		bizName, bizVersion := utils.GetBizNameAndVersionFromUniqueKey(bizStatus.Key)
		vNode.podProvider.DeletePod(ctx, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "not-exist-pod",
				Namespace: corev1.NamespaceDefault,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: bizName,
						Env: []corev1.EnvVar{{
							Name:  "BIZ_VERSION",
							Value: bizVersion,
						}},
					},
				},
			},
		})
	}
}

// Done returns a channel that will be closed when the vnode has exited.
func (vNode *VNode) Done() <-chan struct{} {
	return vNode.done
}

func (vNode *VNode) Exit() <-chan struct{} {
	return vNode.exit
}

// IsLeader returns a bool marked current vnode is leader or not
func (vNode *VNode) IsLeader(clientId string) bool {
	// current time is not after the lease renew time and the lease holder time
	if vNode.lease == nil {
		log.L.Warnf("vnode %s does not have lease", vNode.name)
		return false
	}

	if *vNode.lease.Spec.HolderIdentity == "" {
		log.L.Warnf("vnode %s does not have lease holder", vNode.name)
		return false
	}

	if *vNode.lease.Spec.HolderIdentity != clientId {
		return false
	}

	now := time.Now()
	expiredTime := vNode.lease.Spec.RenewTime.Time.Add(time.Second * model.NodeLeaseDurationSeconds)
	if now.After(expiredTime) {
		log.L.Warnf("vnode %s has expired lease, now is %s , lease is %s , expired at %s", vNode.name, time.Now().Format(time.RFC3339), vNode.lease.Spec.RenewTime.Time.Format(time.RFC3339), expiredTime.Format(time.RFC3339))
		return false
	}

	return true
}

// Err returns err which causes vnode exit
func (vNode *VNode) Err() error {
	return vNode.err
}

// Shutdown is the func of shutting down a vnode when base exit
func (vNode *VNode) Shutdown() {
	select {
	case <-vNode.exit:
	default:
		close(vNode.exit)
	}
}

// LeaderAcquiredByOthers is the func of shutting down a vnode when leader changed
func (vNode *VNode) LeaderAcquiredByOthers() {
	select {
	case vNode.WhenLeaderAcquiredByOthers <- struct{}{}:
	default:
	}
}

func (vNode *VNode) LeaderAcquiredByMe() {
	select {
	case vNode.WhenLeaderAcquiredByMe <- struct{}{}:
	default:
	}
}

func (vNode *VNode) ToDone() {
	select {
	case <-vNode.done:
	default:
		close(vNode.done)
	}
}

//// CheckAndUpdatePodStatus checks and updates a pod in the node
//func (vNode *VNode) CheckAndUpdatePodStatus(ctx context.Context, key string, pod *corev1.Pod) {
//	if vNode.node != nil {
//		vNode.node.PodController().CheckAndUpdatePodStatus(ctx, key, pod)
//	}
//}

// SyncPodsFromKubernetesEnqueue syncs pods from Kubernetes to the node
func (vNode *VNode) SyncPodsFromKubernetesEnqueue(ctx context.Context, key string) {
	if vNode.node != nil {
		vNode.node.PodController().SyncPodsFromKubernetesEnqueue(ctx, key)
	}
}

// DeletePodsFromKubernetesForget deletes pods from Kubernetes in the node
func (vNode *VNode) DeletePodsFromKubernetesForget(ctx context.Context, key string) {
	if vNode.node != nil {
		vNode.node.PodController().DeletePodsFromKubernetesForget(ctx, key)
	}
}

// AddKnowPod stores the pod in the node
func (vNode *VNode) AddKnowPod(pod *corev1.Pod) {
	if vNode.node != nil {
		vNode.node.PodController().AddKnownPod(pod)
	}
}

func (vNode *VNode) GetKnownPod(key string) (any, bool) {
	if vNode.node != nil {
		return vNode.node.PodController().GetKnownPod(key)
	}
	return nil, false
}

// DeleteKnownPod deletes a pod from the node
func (vNode *VNode) DeleteKnownPod(key string) {
	if vNode.node != nil {
		vNode.node.PodController().DeleteKnownPod(key)
	}
}

// NewVNode creates a new virtual node
func NewVNode(config *model.BuildVNodeConfig, tunnel tunnel.Tunnel) (kn *VNode, err error) {
	if config.NodeName == "" {
		return nil, errors.New("node name cannot be empty")
	}
	// Declare variables for nodeProvider and podProvider
	var nodeProvider *VNodeProvider
	var podProvider *VPodProvider

	// Create a new node with the formatted name and configuration
	cm, err := nodeutil2.NewNode(
		config.NodeName,
		// Function to create providers and register the node
		func(cfg nodeutil2.ProviderConfig) (nodeutil2.Provider, node.NodeProvider, error) {
			// Create a new VirtualKubeletNode provider with configuration
			nodeProvider = NewVNodeProvider(config)
			// Initialize pod provider with node namespace, IP, ID, client, and tunnel
			podProvider = NewVPodProvider(cfg.Node.Namespace, config.NodeIP, config.NodeName, config.Client, config.KubeCache, tunnel)

			if err != nil {
				return nil, nil, err
			}
			// Return the providers and nil error if registration is successful
			return podProvider, nodeProvider, nil
		},
		// Function to configure the node
		func(cfg *nodeutil2.NodeConfig) error {
			// Set the node's architecture and operating system
			cfg.Node.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.Node.Status.NodeInfo.OperatingSystem = runtime.GOOS

			// Set the number of workers based on configuration
			cfg.NumWorkers = config.WorkerNum
			return nil
		},
		func(cfg *nodeutil2.NodeConfig) error {
			return buildNode(&cfg.Node, config)
		},
		// Options for creating the node
		nodeutil2.WithClient(config.Client),
		nodeutil2.WithCache(config.KubeCache),
	)
	if err != nil {
		return nil, err
	}

	return &VNode{
		name:                       config.NodeName,
		client:                     config.Client,
		kubeCache:                  config.KubeCache,
		env:                        config.Env,
		nodeProvider:               nodeProvider,
		podProvider:                podProvider,
		vpodType:                   config.VPodType,
		tunnel:                     tunnel,
		node:                       cm,
		exit:                       make(chan struct{}),
		ready:                      make(chan struct{}),
		done:                       make(chan struct{}),
		WhenLeaderAcquiredByMe:     make(chan struct{}, 1),
		WhenLeaderAcquiredByOthers: make(chan struct{}),
		Liveness:                   Liveness{}, // a very old time
	}, nil
}

func buildNode(node *corev1.Node, config *model.BuildVNodeConfig) error {
	oldLabels := node.Labels
	if oldLabels == nil {
		oldLabels = make(map[string]string)
	}
	oldLabels[model.LabelKeyOfBaseName] = config.BaseName
	oldLabels[model.LabelKeyOfBaseClusterName] = config.ClusterName
	oldLabels[model.LabelKeyOfComponent] = model.ComponentVNode
	oldLabels[model.LabelKeyOfEnv] = config.Env
	oldLabels[model.LabelKeyOfBaseVersion] = config.NodeVersion
	oldLabels[corev1.LabelHostname] = config.NodeHostname
	for k, v := range config.CustomLabels {
		oldLabels[k] = v
	}
	node.Labels = oldLabels

	oldAnnotations := node.Annotations
	if oldAnnotations == nil {
		oldAnnotations = make(map[string]string)
	}
	for k, v := range config.CustomAnnotations {
		oldAnnotations[k] = v
	}
	node.Annotations = oldAnnotations

	node.Spec.Taints = append([]corev1.Taint{
		{
			Key:    model.TaintKeyOfVnode,
			Value:  "True",
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    model.TaintKeyOfEnv,
			Value:  config.Env,
			Effect: corev1.TaintEffectNoExecute,
		},
	}, config.CustomTaints...)

	// Set the node status.
	node.Status = corev1.NodeStatus{
		Phase: corev1.NodeRunning,
		Addresses: []corev1.NodeAddress{
			{
				Type:    corev1.NodeInternalIP,
				Address: config.NodeIP,
			},
			{
				Type:    corev1.NodeHostName,
				Address: config.NodeHostname,
			},
		},
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
			{
				Type:   corev1.NodeMemoryPressure,
				Status: corev1.ConditionFalse,
			},
			{
				Type:   corev1.NodeDiskPressure,
				Status: corev1.ConditionFalse,
			},
			{
				Type:   corev1.NodePIDPressure,
				Status: corev1.ConditionFalse,
			},
			{
				Type:   corev1.NodeNetworkUnavailable,
				Status: corev1.ConditionFalse,
			},
		},
		Capacity: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourcePods: resource.MustParse("65535"),
		},
		Allocatable: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourcePods: resource.MustParse("65535"),
		},
	}

	return nil
}
