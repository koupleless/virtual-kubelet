package node

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/java/common"
	"github.com/koupleless/virtual-kubelet/java/model"
	podlet "github.com/koupleless/virtual-kubelet/java/pod/let"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"runtime"
	"time"
)

type KouplelessNode struct {
	config model.BuildKouplelessNodeConfig

	clientSet *kubernetes.Clientset

	vnode       *VirtualKubeletNode
	podProvider *podlet.BaseProvider
	node        *nodeutil.Node

	done  chan struct{}
	ready chan struct{}

	BaseHealthInfoChan chan ark.HealthData
	BaseBizInfoChan    chan []ark.ArkBizInfo
	BaseBizExitChan    chan struct{}

	err error
}

func (n *KouplelessNode) Run(ctx context.Context) {
	var err error

	n.done = make(chan struct{})
	n.ready = make(chan struct{})
	n.BaseBizExitChan = make(chan struct{})
	n.BaseHealthInfoChan = make(chan ark.HealthData, 5)
	n.BaseBizInfoChan = make(chan []ark.ArkBizInfo, 5)

	// process vkNode run and bpc run, catching error
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		n.err = err
		close(n.done)
	}()

	n.clientSet, err = nodeutil.ClientsetFromEnv(n.config.KubeConfigPath)
	if err != nil {
		return
	}

	n.node, err = nodeutil.NewNode(
		n.config.NodeID,
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, node.NodeProvider, error) {
			n.vnode = NewVirtualKubeletNode(model.BuildVirtualNodeConfig{
				NodeIP:    n.config.NodeIP,
				TechStack: n.config.TechStack,
				Version:   n.config.BizVersion,
				BizName:   n.config.BizName,
			})
			// initialize node spec on bootstrap
			n.podProvider = podlet.NewBaseProvider(cfg.Node.Namespace, n.config.NodeIP, n.config.NodeID, n.config.MqttClient, n.clientSet)

			err = n.vnode.Register(context.Background(), cfg.Node)
			if err != nil {
				return nil, nil, err
			}
			return n.podProvider, n.vnode, nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			cfg.InformerResyncPeriod = time.Minute
			cfg.NodeSpec.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.NodeSpec.Status.NodeInfo.OperatingSystem = "linux"
			cfg.DebugHTTP = true

			cfg.NumWorkers = 4
			return nil
		},
		nodeutil.WithClient(n.clientSet),
	)
	if err != nil {
		return
	}

	go n.podProvider.Run(ctx)
	go func() {
		err = n.node.Run(ctx)
		cancel()
	}()

	go n.listenAndSync(ctx)

	go common.TimedTaskWithInterval(ctx, time.Second*9, func(ctx context.Context) {
		n.config.MqttClient.Pub(common.FormatArkletCommandTopic(n.config.NodeID, model.CommandHealth), 1, "{}")
	})

	go common.TimedTaskWithInterval(ctx, time.Second*5, func(ctx context.Context) {
		n.config.MqttClient.Pub(common.FormatArkletCommandTopic(n.config.NodeID, model.CommandQueryAllBiz), 1, "{}")
	})

	// wait ready
	if err = n.node.WaitReady(ctx, time.Minute); err != nil {
		return
	}

	select {
	case <-ctx.Done():
		// exit
		err = errors.Wrap(ctx.Err(), "context canceled")
	case <-n.BaseBizExitChan:
		// base exit, process node delete and pod evict
		err = n.clientSet.CoreV1().Nodes().Delete(ctx, n.vnode.nodeInfo.Name, metav1.DeleteOptions{})
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
	}
}

func (n *KouplelessNode) listenAndSync(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case healthData := <-n.BaseHealthInfoChan:
			go n.vnode.Notify(healthData)
		case bizInfos := <-n.BaseBizInfoChan:
			go n.podProvider.SyncBizInfo(bizInfos)
		}
	}
}

// Done returns a channel that will be closed when the controller has exited.
func (n *KouplelessNode) Done() <-chan struct{} {
	return n.done
}

// Err returns err which causes koupleless node exit
func (n *KouplelessNode) Err() error {
	return n.err
}

func NewKouplelessNode(config *model.BuildKouplelessNodeConfig) (*KouplelessNode, error) {
	if config.MqttClient == nil {
		return nil, errors.New("mqtt client cannot be nil")
	}

	if config.NodeID == "" {
		return nil, errors.New("node name cannot be empty")
	}

	return &KouplelessNode{
		config: *config,
	}, nil
}
