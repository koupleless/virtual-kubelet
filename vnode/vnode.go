package vnode

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	"github.com/koupleless/virtual-kubelet/vnode/node_provider"
	"github.com/koupleless/virtual-kubelet/vnode/pod_provider"
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
	nodeID        string        // Unique identifier of the node
	env           string        // Environment of the node
	client        client.Client // Kubernetes client
	tunnel.Tunnel               // Tunnel for communication with the virtual kubelet

	nodeProvider *node_provider.VNodeProvider // Node provider for the virtual node
	podProvider  *pod_provider.VPodProvider   // Pod provider for the virtual node
	node         *nodeutil.Node               // Node instance for the virtual node

	exit                  chan struct{} // Channel for signaling the node to exit
	ready                 chan struct{} // Channel for signaling the node is ready
	exitWhenLeaderChanged chan struct{} // Channel for signaling the leader has changed
	shouldRetryLease      chan struct{} // Channel for signaling the node to retry lease
	done                  chan struct{} // Channel for signaling the node has exited

	isLeader bool // Flag indicating if the node is the leader

	latestLease *coordinationv1.Lease // Latest lease of the node

	err error // Error that caused the node to exit
}

// Run is the main function for a virtual node
func (n *VNode) Run(ctx context.Context, initData model.NodeInfo) {
	var err error

	// Process the node and catch any errors
	defer func() {
		n.err = err
		close(n.done)
	}()

	// Start the node
	go func() {
		err = n.node.Run(ctx)
	}()

	// Set the node as the leader
	n.isLeader = true
	n.exitWhenLeaderChanged = make(chan struct{})
	n.OnNodeStart(ctx, n.nodeID, initData)
	defer n.OnNodeStop(ctx, n.nodeID)

	// Signal that the node is ready
	close(n.ready)

	// Wait for exit signal
	select {
	case <-ctx.Done():
		// Context canceled, exit
		err = errors.Wrap(ctx.Err(), "context canceled")
	case <-n.exitWhenLeaderChanged:
		// Leader changed, exit
		err = errors.New("leader changed")
	case <-n.exit:
		// Node exit, process node delete and lease delete
		node := n.nodeProvider.CurrNodeInfo()
		err = n.client.Delete(context.Background(), node)
		if err != nil && !apierrors.IsNotFound(err) {
			return
		}
		err = n.client.Delete(context.Background(), n.latestLease)
	}
}

// RenewLease renews the lease of the node
func (n *VNode) RenewLease(ctx context.Context, clientID string) {
	// Delay the first lease update
	time.Sleep(time.Second * model.NodeLeaseUpdatePeriodSeconds)

	// Retry updating the lease
	utils.TimedTaskWithInterval(ctx, time.Second*model.NodeLeaseUpdatePeriodSeconds, func(ctx context.Context) {
		n.retryUpdateLease(ctx, clientID)
	})
}

// retryUpdateLease retries updating the lease of the node
func (n *VNode) retryUpdateLease(ctx context.Context, clientID string) {
	for i := 0; i < 5; i++ {
		lease := &coordinationv1.Lease{}
		err := n.client.Get(ctx, types.NamespacedName{
			Name:      utils.FormatNodeName(n.nodeID, n.env),
			Namespace: corev1.NamespaceNodeLease,
		}, lease)
		if err != nil {
			log.G(ctx).WithError(err).WithField("retries", i).Error("failed to get node lease when updating node lease")
			time.Sleep(time.Millisecond * 200)
			continue
		}

		// If the holder identity is not the current client id, the leader has changed
		if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != clientID {
			n.leaderChanged()
			return
		}

		newLease := lease.DeepCopy()
		newLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
		err = n.client.Update(ctx, newLease)
		if err == nil {
			log.G(ctx).WithField("retries", i).Debug("Successfully updated lease")
			n.latestLease = newLease
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
	n.leaderChanged()
}

// WaitReady waits for the node to be ready
func (n *VNode) WaitReady(ctx context.Context, timeout time.Duration) error {
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	}

	err := n.node.WaitReady(ctx, timeout)
	if err != nil {
		return nil
	}

	// Wait for vnode exist
	utils.CheckAndFinallyCall(ctx, func() bool {
		vnode := &corev1.Node{}
		err = n.client.Get(ctx, types.NamespacedName{
			Name: utils.FormatNodeName(n.nodeID, n.env),
		}, vnode)
		return err == nil
	}, timeout, time.Millisecond*200, func() {}, func() {})

	utils.CheckAndFinallyCall(ctx, func() bool {
		select {
		case <-n.ready:
			return true
		default:
			return false
		}
	}, timeout, time.Millisecond*200, func() {}, func() {})

	return err
}

// CreateNodeLease creates a new lease for the node
func (n *VNode) CreateNodeLease(ctx context.Context, controllerID string) bool {
	// Initialize logger for the context
	logger := log.G(ctx)

	// Defer the function to lock the retry lease to ensure it's always called
	defer n.LockRetryLease()

	// Initialize lease and nodeName
	lease := &coordinationv1.Lease{}
	nodeName := utils.FormatNodeName(n.nodeID, n.env)

	// Attempt to get the lease from the client
	err := n.client.Get(ctx, types.NamespacedName{
		Name:      nodeName,
		Namespace: corev1.NamespaceNodeLease,
	}, lease)

	// If there's an error, check if it's because the lease was not found
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If not found, try to create a new lease
			lease = n.newLease(controllerID)

			// Attempt to create the lease
			err = n.client.Create(ctx, lease)
			// If the context is canceled or deadline exceeded, return false
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}

			// If the lease already exists, return false
			if apierrors.IsAlreadyExists(err) {
				return false
			} else if err != nil {
				// Log the error if there's a problem creating the lease
				logger.WithError(err).Error("error creating node lease")
				return false
			}
		}
	} else {
		// If the lease exists, check if it's outdated
		if lease.Spec.RenewTime == nil || time.Now().Sub(lease.Spec.RenewTime.Time).Microseconds() > model.NodeLeaseDurationSeconds*1000*1000 {
			// If outdated, try to update the lease
			newLease := lease.DeepCopy()
			newLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			newLease.Spec.HolderIdentity = ptr.To(controllerID)
			err = n.client.Update(ctx, newLease)
			// If the update is successful, return true
			if err == nil {
				return true
			}
			// Log the error if there's a problem updating the lease
			logger.WithError(err).Error("failed to update outdated node lease")
		}
		// If the lease is not outdated, return false
		return false
	}

	// Update the latest lease
	n.latestLease = lease
	return true
}

// newLease creates a new lease for the node
func (n *VNode) newLease(holderIdentity string) *coordinationv1.Lease {
	nodeName := utils.FormatNodeName(n.nodeID, n.env)
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: corev1.NamespaceNodeLease,
			Labels: map[string]string{
				model.LabelKeyOfEnv:       n.env,
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
func (n *VNode) SyncNodeStatus(data model.NodeStatusData) {
	if n.nodeProvider != nil {
		go n.nodeProvider.Notify(data)
	}
}

// SyncAllContainerInfo syncs the status of all containers
func (n *VNode) SyncAllContainerInfo(ctx context.Context, infos []model.ContainerStatusData) {
	if n.podProvider != nil {
		go n.podProvider.SyncAllContainerInfo(ctx, infos)
	}
}

// SyncOneContainerInfo syncs the status of a single container
func (n *VNode) SyncOneContainerInfo(ctx context.Context, info model.ContainerStatusData) {
	if n.podProvider != nil {
		go n.podProvider.SyncOneContainerInfo(ctx, info)
	}
}

// InitContainerInfo initializes the status of a container
func (n *VNode) InitContainerInfo(info model.ContainerStatusData) {
	if n.podProvider != nil {
		n.podProvider.InitContainerInfo(info)
	}
}

// Done returns a channel that will be closed when the vnode has exited.
func (n *VNode) Done() <-chan struct{} {
	return n.done
}

// ExitWhenLeaderChanged returns a channel that will be closed when the vnode leader changed
func (n *VNode) ExitWhenLeaderChanged() <-chan struct{} {
	return n.exitWhenLeaderChanged
}

// IsLeader returns a bool marked current vnode is leader or not
func (n *VNode) IsLeader() bool {
	return n.isLeader
}

// ShouldRetryLease returns a channel that will be closed when the vnode should retry lease
func (n *VNode) ShouldRetryLease() <-chan struct{} {
	return n.shouldRetryLease
}

// RetryLease signals the vnode to retry the lease
func (n *VNode) RetryLease() {
	select {
	case <-n.shouldRetryLease:
	default:
		close(n.shouldRetryLease)
	}
}

// LockRetryLease locks the vnode to retry the lease
func (n *VNode) LockRetryLease() {
	n.shouldRetryLease = make(chan struct{})
}

// Err returns err which causes vnode exit
func (n *VNode) Err() error {
	return n.err
}

// Shutdown is the func of shutting down a vnode when base exit
func (n *VNode) Shutdown() {
	select {
	case <-n.exit:
	default:
		close(n.exit)
	}
}

// leaderChanged is the func of shutting down a vnode when leader changed
func (n *VNode) leaderChanged() {
	select {
	case <-n.exitWhenLeaderChanged:
	default:
		close(n.exitWhenLeaderChanged)
	}
}

// PodStore stores the pod in the node
func (n *VNode) PodStore(key string, pod *corev1.Pod) {
	if n.node != nil {
		n.node.PodController().StorePod(key, pod)
	}
}

// LoadPodFromController loads a pod from the node's pod controller
func (n *VNode) LoadPodFromController(key string) (any, bool) {
	return n.node.PodController().LoadPod(key)
}

// CheckAndUpdatePod checks and updates a pod in the node
func (n *VNode) CheckAndUpdatePod(ctx context.Context, key string, obj interface{}, pod *corev1.Pod) {
	if n.node != nil {
		n.node.PodController().CheckAndUpdatePod(ctx, key, obj, pod)
	}
}

// SyncPodsFromKubernetesEnqueue syncs pods from Kubernetes to the node
func (n *VNode) SyncPodsFromKubernetesEnqueue(ctx context.Context, key string) {
	if n.node != nil {
		n.node.PodController().SyncPodsFromKubernetesEnqueue(ctx, key)
	}
}

// DeletePodsFromKubernetesForget deletes pods from Kubernetes in the node
func (n *VNode) DeletePodsFromKubernetesForget(ctx context.Context, key string) {
	if n.node != nil {
		n.node.PodController().DeletePodsFromKubernetesForget(ctx, key)
	}
}

// DeletePod deletes a pod from the node
func (n *VNode) DeletePod(key string) {
	if n.node != nil {
		n.node.PodController().DeletePod(key)
	}
}

// NewVNode creates a new virtual node
func NewVNode(config *model.BuildVNodeConfig, t tunnel.Tunnel) (kn *VNode, err error) {
	if t == nil {
		return nil, errors.New("tunnel provider be nil")
	}

	if config.NodeID == "" {
		return nil, errors.New("node id cannot be empty")
	}
	// Declare variables for nodeProvider and podProvider
	var nodeProvider *node_provider.VNodeProvider
	var podProvider *pod_provider.VPodProvider

	// Create a new node with the formatted name and configuration
	cm, err := nodeutil.NewNode(
		utils.FormatNodeName(config.NodeID, config.Env),
		// Function to create providers and register the node
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, virtual_kubelet.NodeProvider, error) {
			// Create a new VirtualKubeletNode provider with configuration
			nodeProvider = node_provider.NewVirtualKubeletNode(model.BuildVNodeProviderConfig{
				NodeIP:            config.NodeIP,
				NodeHostname:      config.NodeHostname,
				Version:           config.NodeVersion,
				Name:              config.NodeName,
				Env:               config.Env,
				CustomTaints:      config.CustomTaints,
				CustomAnnotations: config.CustomAnnotations,
				CustomLabels:      config.CustomLabels,
			})
			// Initialize pod provider with node namespace, IP, ID, client, and tunnel
			podProvider = pod_provider.NewVPodProvider(cfg.Node.Namespace, config.NodeIP, config.NodeID, config.Client, t)

			// Register the node with the tunnel key
			err = nodeProvider.Register(cfg.Node, t.Key())
			if err != nil {
				return nil, nil, err
			}
			// Return the providers and nil error if registration is successful
			return podProvider, nodeProvider, nil
		},
		// Function to configure the node
		func(cfg *nodeutil.NodeConfig) error {
			// Set the node's architecture and operating system
			cfg.NodeSpec.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.NodeSpec.Status.NodeInfo.OperatingSystem = runtime.GOOS

			// Set the number of workers based on configuration
			cfg.NumWorkers = config.WorkerNum
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
		nodeID:                config.NodeID,
		client:                config.Client,
		env:                   config.Env,
		nodeProvider:          nodeProvider,
		podProvider:           podProvider,
		Tunnel:                t,
		node:                  cm,
		exit:                  make(chan struct{}),
		ready:                 make(chan struct{}),
		done:                  make(chan struct{}),
		exitWhenLeaderChanged: make(chan struct{}),
		shouldRetryLease:      make(chan struct{}),
	}, nil
}
