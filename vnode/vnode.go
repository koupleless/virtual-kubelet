package vnode

import (
	"context"
	"fmt"
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
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type VNode struct {
	nodeID string
	env    string
	client client.Client
	tunnel.Tunnel

	nodeProvider *node_provider.VNodeProvider
	podProvider  *pod_provider.VPodProvider
	node         *nodeutil.Node

	exit                  chan struct{}
	ready                 chan struct{}
	exitWhenLeaderChanged chan struct{}
	shouldRetryLease      chan struct{}
	done                  chan struct{}

	isLeader bool

	latestLease *coordinationv1.Lease

	err error
}

func (n *VNode) Run(ctx context.Context, initData model.NodeInfo) {
	var err error

	// process vkNode run and bpc run, catching error
	defer func() {
		n.err = err
		close(n.done)
	}()

	go func() {
		err = n.node.Run(ctx)
	}()

	n.isLeader = true
	n.exitWhenLeaderChanged = make(chan struct{})
	n.OnNodeStart(ctx, n.nodeID, initData)
	defer n.OnNodeStop(ctx, n.nodeID)

	close(n.ready)

	select {
	case <-ctx.Done():
		// exit
		err = errors.Wrap(ctx.Err(), "context canceled")
	case <-n.exitWhenLeaderChanged:
		// exit local go runtime
		err = errors.New("leader changed")
	case <-n.exit:
		// node exit, process node delete and lease delete
		node := n.nodeProvider.CurrNodeInfo()
		err = n.client.Delete(context.Background(), node)
		if err != nil && !apierrors.IsNotFound(err) {
			return
		}
		err = n.client.Delete(context.Background(), n.latestLease)
	}
}

func (n *VNode) RenewLease(ctx context.Context, clientID string) {
	// first lease update delay NodeLeaseUpdatePeriodSeconds
	time.Sleep(time.Second * model.NodeLeaseUpdatePeriodSeconds)

	utils.TimedTaskWithInterval(ctx, time.Second*model.NodeLeaseUpdatePeriodSeconds, func(ctx context.Context) {
		n.retryUpdateLease(ctx, clientID)
	})
}

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

		// if holder identity not current client id, means leader changed
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

	// wait for vnode exist
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

func (n *VNode) CreateNodeLease(ctx context.Context, controllerID string) bool {
	logger := log.G(ctx)

	defer n.LockRetryLease()

	lease := &coordinationv1.Lease{}
	nodeName := utils.FormatNodeName(n.nodeID, n.env)

	err := n.client.Get(ctx, types.NamespacedName{
		Name:      nodeName,
		Namespace: corev1.NamespaceNodeLease,
	}, lease)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// try to create new lease
			lease = n.newLease(controllerID)

			err = n.client.Create(ctx, lease)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// context canceled, just return
				return false
			}

			if apierrors.IsAlreadyExists(err) {
				return false
			} else if err != nil {
				logger.WithError(err).Error("error creating node lease")
				return false
			}
		}
	} else {
		// lease exist, check if outdated
		if lease.Spec.RenewTime == nil || time.Now().Sub(lease.Spec.RenewTime.Time).Microseconds() > model.NodeLeaseDurationSeconds*1000*1000 {
			// outdated, try to update
			newLease := lease.DeepCopy()
			newLease.Spec.RenewTime = &metav1.MicroTime{Time: time.Now()}
			newLease.Spec.HolderIdentity = ptr.To(controllerID)
			err = n.client.Update(ctx, newLease)
			if err == nil {
				return true
			}
			logger.WithError(err).Error("failed to update outdated node lease")
		}
		// not outdated
		return false
	}

	n.latestLease = lease
	return true
}

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

func (n *VNode) SyncNodeStatus(data model.NodeStatusData) {
	if n.nodeProvider != nil {
		go n.nodeProvider.Notify(data)
	}
}

func (n *VNode) SyncAllContainerInfo(ctx context.Context, infos []model.ContainerStatusData) {
	if n.podProvider != nil {
		go n.podProvider.SyncAllContainerInfo(ctx, infos)
	}
}

func (n *VNode) SyncOneContainerInfo(ctx context.Context, info model.ContainerStatusData) {
	if n.podProvider != nil {
		go n.podProvider.SyncOneContainerInfo(ctx, info)
	}
}

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

func (n *VNode) ShouldRetryLease() <-chan struct{} {
	return n.shouldRetryLease
}

func (n *VNode) RetryLease() {
	select {
	case <-n.shouldRetryLease:
	default:
		close(n.shouldRetryLease)
	}
}

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

func (n *VNode) PodStore(key string, pod *corev1.Pod) {
	if n.node != nil {
		n.node.PodController().StorePod(key, pod)
	}
}

func (n *VNode) LoadPodFromController(key string) (any, bool) {
	return n.node.PodController().LoadPod(key)
}

func (n *VNode) CheckAndUpdatePod(ctx context.Context, key string, obj interface{}, pod *corev1.Pod) {
	if n.node != nil {
		n.node.PodController().CheckAndUpdatePod(ctx, key, obj, pod)
	}
}

func (n *VNode) SyncPodsFromKubernetesEnqueue(ctx context.Context, key string) {
	if n.node != nil {
		n.node.PodController().SyncPodsFromKubernetesEnqueue(ctx, key)
	}
}

func (n *VNode) DeletePodsFromKubernetesForget(ctx context.Context, key string) {
	if n.node != nil {
		n.node.PodController().DeletePodsFromKubernetesForget(ctx, key)
	}
}

func (n *VNode) DeletePod(key string) {
	if n.node != nil {
		n.node.PodController().DeletePod(key)
	}
}

func NewVNode(config *model.BuildVNodeConfig, t tunnel.Tunnel) (kn *VNode, err error) {
	if t == nil {
		return nil, errors.New("tunnel provider be nil")
	}

	if config.NodeID == "" {
		return nil, errors.New("node id cannot be empty")
	}
	var nodeProvider *node_provider.VNodeProvider
	var podProvider *pod_provider.VPodProvider
	cm, err := nodeutil.NewNode(
		utils.FormatNodeName(config.NodeID, config.Env),
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, virtual_kubelet.NodeProvider, error) {
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
			// initialize node spec on bootstrap
			podProvider = pod_provider.NewVPodProvider(cfg.Node.Namespace, config.NodeIP, config.NodeID, config.Client, t)

			err = nodeProvider.Register(cfg.Node, t.Key())
			if err != nil {
				return nil, nil, err
			}
			return podProvider, nodeProvider, nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			cfg.NodeSpec.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.NodeSpec.Status.NodeInfo.OperatingSystem = runtime.GOOS

			cfg.NumWorkers = config.WorkerNum
			return nil
		},
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
