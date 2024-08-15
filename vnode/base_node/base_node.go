package base_node

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	"github.com/koupleless/virtual-kubelet/vnode/base_node/node_provider"
	"github.com/koupleless/virtual-kubelet/vnode/base_node/pod_provider"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"runtime"
	"time"
)

type BaseNode struct {
	nodeID    string
	clientSet kubernetes.Interface
	tunnel    tunnel.Tunnel

	nodeProvider *node_provider.BaseNodeProvider
	podProvider  *pod_provider.BaseProvider
	node         *nodeutil.Node

	done chan struct{}
	exit chan struct{}

	err error
}

func (n *BaseNode) Run(ctx context.Context) {
	var err error

	// process vkNode run and bpc run, catching error
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		n.err = err
		close(n.done)
	}()

	n.podProvider.Run(ctx)
	go func() {
		err = n.node.Run(ctx)
		cancel()
	}()

	n.tunnel.OnNodeStart(ctx, n.nodeID)

	go utils.TimedTaskWithInterval(ctx, time.Second*9, func(ctx context.Context) {
		err = n.tunnel.FetchHealthData(ctx, n.nodeID)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Failed to fetch health info from %s", n.nodeID)
		}
	})

	go utils.TimedTaskWithInterval(ctx, time.Second*5, func(ctx context.Context) {
		err = n.tunnel.QueryAllContainerStatusData(ctx, n.nodeID)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("Failed to query biz info from %s", n.nodeID)
		}
	})

	select {
	case <-ctx.Done():
		// exit
		err = errors.Wrap(ctx.Err(), "context canceled")
	case <-n.exit:
		// base exit, process node delete and pod evict
		err = n.clientSet.CoreV1().Nodes().Delete(ctx, n.nodeProvider.CurrNodeInfo().Name, metav1.DeleteOptions{})
		if err != nil {
			err = errors.Wrap(err, "error deleting base biz node")
			return
		}
		pods, err := n.podProvider.GetPods(ctx)
		if err != nil {
			err = errors.Wrap(err, "error getting pods from provider")
			return
		}
		for _, pod := range pods {
			// base exit, process node delete and pod evict
			err = n.clientSet.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{
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

func (n *BaseNode) WaitReady(ctx context.Context, timeout time.Duration) error {
	return n.node.WaitReady(ctx, timeout)
}

func (n *BaseNode) SyncNodeStatus(data model.NodeStatusData) {
	go n.nodeProvider.Notify(data)
}

func (n *BaseNode) SyncAllContainerInfo(infos []model.ContainerStatusData) {
	go n.podProvider.SyncContainerInfo(context.Background(), infos)
}

func (n *BaseNode) SyncBizInfo(info model.ContainerStatusData) {
	go n.podProvider.SyncSingleContainerInfo(context.Background(), info)
}

// Done returns a channel that will be closed when the controller has exited.
func (n *BaseNode) Done() <-chan struct{} {
	return n.done
}

// Err returns err which causes koupleless node exit
func (n *BaseNode) Err() error {
	return n.err
}

func (n *BaseNode) Exit() {
	select {
	case <-n.exit:
	default:
		close(n.exit)
	}
}

func (n *BaseNode) PodStore(key string, pod *corev1.Pod) {
	n.node.PodController().StorePod(key, pod)
}

func (n *BaseNode) LoadPodFromController(key string) (any, bool) {
	return n.node.PodController().LoadPod(key)
}

func (n *BaseNode) CheckAndUpdatePod(ctx context.Context, key string, obj interface{}, pod *corev1.Pod) {
	n.node.PodController().CheckAndUpdatePod(ctx, key, obj, pod)
}

func (n *BaseNode) SyncPodsFromKubernetesEnqueue(ctx context.Context, key string) {
	n.node.PodController().SyncPodsFromKubernetesEnqueue(ctx, key)
}

func (n *BaseNode) DeletePodsFromKubernetesForget(ctx context.Context, key string) {
	n.node.PodController().DeletePodsFromKubernetesForget(ctx, key)
}

func (n *BaseNode) DeletePod(key string) {
	n.node.PodController().DeletePod(key)
}

func NewBaseNode(config *BuildBaseNodeConfig) (kn *BaseNode, err error) {
	if config.Tunnel == nil {
		return nil, errors.New("tunnel provider be nil")
	}

	if config.BaseID == "" {
		return nil, errors.New("node name cannot be empty")
	}
	var nodeProvider *node_provider.BaseNodeProvider
	var podProvider *pod_provider.BaseProvider
	cm, err := nodeutil.NewNode(
		utils.FormatBaseNodeName(config.BaseID),
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, virtual_kubelet.NodeProvider, error) {
			nodeProvider = node_provider.NewVirtualKubeletNode(node_provider.BuildBaseNodeProviderConfig{
				NodeIP:       config.NodeIP,
				NodeHostname: config.NodeHostname,
				Version:      config.NodeVersion,
				Name:         config.NodeName,
				Env:          config.Env,
			})
			// initialize node spec on bootstrap
			podProvider = pod_provider.NewBaseProvider(cfg.Node.Namespace, config.NodeIP, config.BaseID, config.KubeClient, config.Tunnel)

			err = nodeProvider.Register(cfg.Node, config.Tunnel.Key())
			if err != nil {
				return nil, nil, err
			}
			return podProvider, nodeProvider, nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			cfg.NodeSpec.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.NodeSpec.Status.NodeInfo.OperatingSystem = "linux"

			cfg.NumWorkers = 1
			return nil
		},
		nodeutil.WithClient(config.KubeClient),
		nodeutil.WithPodLister(config.PodLister),
		nodeutil.WithPodInformer(config.PodInformer),
	)
	if err != nil {
		return nil, err
	}

	return &BaseNode{
		nodeID:       config.BaseID,
		clientSet:    config.KubeClient,
		nodeProvider: nodeProvider,
		podProvider:  podProvider,
		tunnel:       config.Tunnel,
		node:         cm,
		done:         make(chan struct{}),
		exit:         make(chan struct{}),
	}, nil
}
