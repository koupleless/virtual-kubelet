package provider

import (
	"context"
	"fmt"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"time"

	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	"github.com/pkg/errors"
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

	nodeProvider *VNodeProvider // Node provider for the virtual node
	podProvider  *VPodProvider  // Pod provider for the virtual node
	node         *nodeutil.Node // Node instance for the virtual node
	tunnel       tunnel.Tunnel

	exit                  chan struct{} // Channel for signaling the node to exit
	ready                 chan struct{} // Channel for signaling the node is ready
	exitWhenLeaderChanged chan struct{} // Channel for signaling the leader has changed
	done                  chan struct{} // Channel for signaling the node has exited

	lease    *coordinationv1.Lease // Latest lease of the node
	Liveness Liveness              // Liveness of the node from provider

	err error // Error that caused the node to exit
}

func (vNode *VNode) GetNodeName() string {
	return vNode.name
}

func (vNode *VNode) GetLease() *coordinationv1.Lease {
	return vNode.lease
}

// Run is the main function for a virtual node
func (vNode *VNode) Run(ctx context.Context, initData model.NodeInfo) {
	var err error

	// Process the node and catch any errors
	defer func() {
		vNode.err = err
		close(vNode.done)
	}()

	// Start the node
	go func() {
		err = vNode.node.Run(ctx)
	}()

	// Set the node as the leader
	vNode.exitWhenLeaderChanged = make(chan struct{})
	// TODO: remove the dep of tunnel
	vNode.tunnel.RegisterNode(ctx, initData)
	defer vNode.tunnel.UnRegisterNode(ctx, vNode.name)

	// Signal that the node is ready
	close(vNode.ready)

	// Wait for exit signal
	select {
	case <-ctx.Done():
		// Context canceled, exit
		err = errors.Wrap(ctx.Err(), "context canceled")
	case <-vNode.exitWhenLeaderChanged:
		// Leader changed, exit
		err = errors.New("leader changed")
	case <-vNode.exit:
		// Node exit, process node delete and lease delete
		node := vNode.nodeProvider.CurrNodeInfo()
		err = vNode.client.Delete(context.Background(), node)
		if err != nil && !apierrors.IsNotFound(err) {
			return
		}
		err = vNode.client.Delete(context.Background(), vNode.lease)
	}
}

// StartLeaderElection renews the lease of the node
func (vNode *VNode) StartLeaderElection(ctx context.Context, clientID string) {
	vNode.createOrRetryUpdateLease(ctx, clientID)
	// Retry updating the lease
	utils.TimedTaskWithInterval(ctx, time.Second*model.NodeLeaseUpdatePeriodSeconds, func(ctx context.Context) {
		vNode.createOrRetryUpdateLease(ctx, clientID)
	})
}

// createOrRetryUpdateLease retries updating the lease of the node
func (vNode *VNode) createOrRetryUpdateLease(ctx context.Context, clientID string) {
	for i := 0; i < model.NodeLeaseMaxRetryTimes; i++ {
		lease := &coordinationv1.Lease{}
		err := vNode.client.Get(ctx, types.NamespacedName{
			Name:      vNode.name,
			Namespace: corev1.NamespaceNodeLease,
		}, lease)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// If not found, try to create a new lease
				lease = vNode.newLease(clientID)

				// Attempt to create the lease
				err = vNode.client.Create(ctx, lease)
				// If the context is canceled or deadline exceeded, return false
				if err != nil {
					// Log the error if there's a problem creating the lease
					log.G(ctx).WithError(err).Errorf("node lease %s creating error", vNode.name)
				}
				continue
			}

			log.G(ctx).WithError(err).WithField("retries", i).Error("failed to get node lease when updating node lease")
			time.Sleep(time.Millisecond * 200)
			continue
		}

		vNode.lease = lease
		// If the holder identity is not the current client id, the leader has changed
		if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != clientID {
			vNode.leaderChanged()
			return
		}

		newLease := lease.DeepCopy()
		newLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
		err = vNode.client.Update(ctx, newLease)
		if err == nil {
			log.G(ctx).WithField("retries", i).Debug("Successfully updated lease")
			vNode.lease = newLease
			return
		}
		log.G(ctx).WithError(err).Error("failed to update node lease")
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		// OptimisticLockError requires getting the newer version of lease to proceed.
		if apierrors.IsConflict(err) {
			continue
		}
	}

	log.G(ctx).WithError(fmt.Errorf("failed after %d attempts to update node lease", model.NodeLeaseMaxRetryTimes)).Error("failed to update node lease")
}

// WaitReady waits for the node to be ready
func (vNode *VNode) WaitReady(ctx context.Context, timeout time.Duration) error {
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	}

	err := vNode.node.WaitReady(ctx, timeout)
	if err != nil {
		return nil
	}

	// Wait for vnode exist
	utils.CheckAndFinallyCall(ctx, func() bool {
		vnode := &corev1.Node{}
		err = vNode.client.Get(ctx, types.NamespacedName{
			Name: vNode.name,
		}, vnode)
		return err == nil
	}, timeout, time.Millisecond*200, func() {}, func() {})

	utils.CheckAndFinallyCall(ctx, vNode.IsReady, timeout, time.Millisecond*200, func() {}, func() {})

	return err
}

func (vNode *VNode) IsReady() bool {
	select {
	case <-vNode.ready:
		return true
	default:
		return false
	}
}

// newLease creates a new lease for the node
func (vNode *VNode) newLease(holderIdentity string) *coordinationv1.Lease {
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
	if utils.NodeStatusEqual(vNode.nodeProvider.latestNodeStatusData, data) {
		return
	}
	if vNode.nodeProvider != nil {
		vNode.nodeProvider.Notify(data)
	}
}

// SyncAllContainerInfo syncs the status of all containers
func (vNode *VNode) SyncAllContainerInfo(ctx context.Context, infos []model.BizStatusData) {
	if vNode.podProvider != nil {
		vNode.podProvider.SyncAllBizStatusToKube(ctx, infos)
	}
}

// SyncOneContainerInfo syncs the status of a single container
func (vNode *VNode) SyncOneContainerInfo(ctx context.Context, bizStatusData model.BizStatusData) {
	if vNode.podProvider != nil {
		vNode.podProvider.SyncBizStatusToKube(ctx, bizStatusData)
	}
}

// Done returns a channel that will be closed when the vnode has exited.
func (vNode *VNode) Done() <-chan struct{} {
	return vNode.done
}

// ExitWhenLeaderChanged returns a channel that will be closed when the vnode leader changed
func (vNode *VNode) ExitWhenLeaderChanged() <-chan struct{} {
	return vNode.exitWhenLeaderChanged
}

// IsLeader returns a bool marked current vnode is leader or not
func (vNode *VNode) IsLeader(clientId string) bool {
	// current time is not after the lease renew time and the lease holder time

	return vNode.lease != nil && *vNode.lease.Spec.HolderIdentity == clientId &&
		!time.Now().After(vNode.lease.Spec.RenewTime.Time.Add(time.Second*model.NodeLeaseDurationSeconds))
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

// leaderChanged is the func of shutting down a vnode when leader changed
func (vNode *VNode) leaderChanged() {
	select {
	case <-vNode.exitWhenLeaderChanged:
	default:
		close(vNode.exitWhenLeaderChanged)
	}
}

// AddKnowPod stores the pod in the node
func (vNode *VNode) AddKnowPod(pod *corev1.Pod) {
	if vNode.node != nil {
		vNode.node.PodController().AddKnownPod(pod)
	}
}

// CheckAndUpdatePodStatus checks and updates a pod in the node
func (vNode *VNode) CheckAndUpdatePodStatus(ctx context.Context, key string, pod *corev1.Pod) {
	if vNode.node != nil {
		vNode.node.PodController().CheckAndUpdatePodStatus(ctx, key, pod)
	}
}

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
	cm, err := nodeutil.NewNode(
		config.NodeName,
		// Function to create providers and register the node
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, virtual_kubelet.NodeProvider, error) {
			// Create a new VirtualKubeletNode provider with configuration
			nodeProvider = NewVNodeProvider(config)
			// Initialize pod provider with node namespace, IP, ID, client, and tunnel
			podProvider = NewVPodProvider(cfg.Node.Namespace, config.NodeIP, config.NodeName, config.Client, tunnel)

			// Register the node with the tunnel key
			err = nodeProvider.Register(cfg.Node)
			if err != nil {
				return nil, nil, err
			}
			// Return the providers and nil error if registration is successful
			return podProvider, nodeProvider, nil
		},
		// Function to configure the node
		func(cfg *nodeutil.NodeConfig) error {
			// Set the node's architecture and operating system
			cfg.Node.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.Node.Status.NodeInfo.OperatingSystem = runtime.GOOS

			// Set the number of workers based on configuration
			cfg.NumWorkers = config.WorkerNum
			return nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			oldLabels := cfg.Node.Labels
			oldLabels[model.LabelKeyOfVNodeName] = config.NodeName
			oldLabels[model.LabelKeyOfVNodeClusterName] = config.ClusterName
			oldLabels[model.LabelKeyOfComponent] = model.ComponentVNode
			oldLabels[model.LabelKeyOfEnv] = config.Env
			oldLabels[model.LabelKeyOfVNodeVersion] = config.NodeVersion
			oldLabels[corev1.LabelHostname] = config.NodeHostname
			cfg.Node.Labels = oldLabels
			return nil
		},
		// Options for creating the node
		nodeutil.WithClient(config.Client),
		nodeutil.WithCache(config.KubeCache),
	)
	if err != nil {
		return nil, err
	}

	return &VNode{
		name:                  config.NodeName,
		client:                config.Client,
		kubeCache:             config.KubeCache,
		env:                   config.Env,
		nodeProvider:          nodeProvider,
		podProvider:           podProvider,
		tunnel:                tunnel,
		node:                  cm,
		exit:                  make(chan struct{}),
		ready:                 make(chan struct{}),
		done:                  make(chan struct{}),
		exitWhenLeaderChanged: make(chan struct{}),
		Liveness:              Liveness{}, // a very old time
	}, nil
}
