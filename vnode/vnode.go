package vnode

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	"github.com/koupleless/virtual-kubelet/vnode/node_provider"
	"github.com/koupleless/virtual-kubelet/vnode/pod_provider"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type VNode struct {
	nodeID string
	client client.Client
	tunnel tunnel.Tunnel

	nodeProvider *node_provider.VNodeProvider
	podProvider  *pod_provider.VPodProvider
	node         *nodeutil.Node

	done chan struct{}
	exit chan struct{}

	err error
}

func (n *VNode) Run(ctx context.Context) {
	var err error

	// process vkNode run and bpc run, catching error
	defer func() {
		n.err = err
		close(n.done)
	}()

	n.podProvider.Run(ctx)
	go func() {
		err = n.node.Run(ctx)
	}()

	select {
	case <-ctx.Done():
		// exit
		err = errors.Wrap(ctx.Err(), "context canceled")
	case <-n.exit:
		// base exit, process node delete and pod evict
		err = n.client.Delete(ctx, n.nodeProvider.CurrNodeInfo(), &client.DeleteOptions{})
		if err != nil {
			err = errors.Wrap(err, "error deleting vnode")
			return
		}
		pods, err := n.podProvider.GetPods(ctx)
		if err != nil {
			err = errors.Wrap(err, "error getting pods from provider")
			return
		}
		for _, pod := range pods {
			// base exit, process node delete and pod evict
			err = n.client.Delete(ctx, pod, &client.DeleteOptions{
				GracePeriodSeconds: ptr.To[int64](0),
			})
			if err != nil {
				err = errors.Wrap(err, "error deleting pod")
				return
			}
		}
		n.tunnel.OnNodeStop(ctx, n.nodeID)
	}
}

func (n *VNode) WaitReady(timeout time.Duration) error {
	ctx := context.Background()
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
			Name: utils.FormatNodeName(n.nodeID),
		}, vnode)
		return err == nil
	}, timeout, time.Millisecond*200, func() {}, func() {})

	return err
}

func (n *VNode) SyncNodeStatus(data model.NodeStatusData) {
	go n.nodeProvider.Notify(data)
}

func (n *VNode) SyncAllContainerInfo(infos []model.ContainerStatusData) {
	go n.podProvider.SyncContainerInfo(context.Background(), infos)
}

func (n *VNode) SyncSingleContainerInfo(info model.ContainerStatusData) {
	go n.podProvider.SyncSingleContainerInfo(context.Background(), info)
}

// Done returns a channel that will be closed when the vnode has exited.
func (n *VNode) Done() <-chan struct{} {
	return n.done
}

// Err returns err which causes vnode exit
func (n *VNode) Err() error {
	return n.err
}

// Shutdown is the func of shutting down a vnode
func (n *VNode) Shutdown() {
	select {
	case <-n.exit:
	default:
		close(n.exit)
	}
}

func (n *VNode) PodStore(key string, pod *corev1.Pod) {
	n.node.PodController().StorePod(key, pod)
}

func (n *VNode) LoadPodFromController(key string) (any, bool) {
	return n.node.PodController().LoadPod(key)
}

func (n *VNode) CheckAndUpdatePod(ctx context.Context, key string, obj interface{}, pod *corev1.Pod) {
	n.node.PodController().CheckAndUpdatePod(ctx, key, obj, pod)
}

func (n *VNode) SyncPodsFromKubernetesEnqueue(ctx context.Context, key string) {
	n.node.PodController().SyncPodsFromKubernetesEnqueue(ctx, key)
}

func (n *VNode) DeletePodsFromKubernetesForget(ctx context.Context, key string) {
	n.node.PodController().DeletePodsFromKubernetesForget(ctx, key)
}

func (n *VNode) DeletePod(key string) {
	n.node.PodController().DeletePod(key)
}

func NewVNode(config *model.BuildVNodeConfig, t tunnel.Tunnel) (kn *VNode, err error) {
	if t == nil {
		return nil, errors.New("tunnel provider be nil")
	}

	if config.NodeID == "" {
		return nil, errors.New("node name cannot be empty")
	}
	var nodeProvider *node_provider.VNodeProvider
	var podProvider *pod_provider.VPodProvider
	cm, err := nodeutil.NewNode(
		utils.FormatNodeName(config.NodeID),
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, virtual_kubelet.NodeProvider, error) {
			nodeProvider = node_provider.NewVirtualKubeletNode(model.BuildVNodeProviderConfig{
				NodeIP:       config.NodeIP,
				NodeHostname: config.NodeHostname,
				Version:      config.NodeVersion,
				Name:         config.NodeName,
				Env:          config.Env,
				CustomTaints: config.CustomTaints,
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

			cfg.NumWorkers = 1
			return nil
		},
		nodeutil.WithClient(config.Client),
		nodeutil.WithCache(config.KubeCache),
	)
	if err != nil {
		return nil, err
	}

	return &VNode{
		nodeID:       config.NodeID,
		client:       config.Client,
		nodeProvider: nodeProvider,
		podProvider:  podProvider,
		tunnel:       t,
		node:         cm,
		done:         make(chan struct{}),
		exit:         make(chan struct{}),
	}, nil
}
