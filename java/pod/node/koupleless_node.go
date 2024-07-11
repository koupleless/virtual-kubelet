package node

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/java/common"
	"github.com/koupleless/virtual-kubelet/java/model"
	podlet "github.com/koupleless/virtual-kubelet/java/pod/let"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
	"runtime"
	"time"
)

type KouplelessNode struct {
	clientSet  *kubernetes.Clientset
	mqttClient *mqtt.Client
	nodeID     string

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
	// process vkNode run and bpc run, catching error
	var err error
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		n.err = err
		close(n.done)
	}()

	go n.podProvider.Run(ctx)
	go func() {
		err = n.node.Run(ctx)
		cancel()
	}()

	go n.listenAndSync(ctx)

	go common.TimedTaskWithInterval(ctx, time.Second*9, func(ctx context.Context) {
		n.mqttClient.Pub(common.FormatArkletCommandTopic(n.nodeID, model.CommandHealth), 0, "{}")
	})

	go common.TimedTaskWithInterval(ctx, time.Second*5, func(ctx context.Context) {
		n.mqttClient.Pub(common.FormatArkletCommandTopic(n.nodeID, model.CommandQueryAllBiz), 0, "{}")
	})

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

// WaitReady waits for the specified timeout for the controller to be ready.
//
// The timeout is for convenience so the caller doesn't have to juggle an extra context.
func (n *KouplelessNode) WaitReady(ctx context.Context, timeout time.Duration) error {
	// complete pod health check
	return n.node.WaitReady(ctx, timeout)
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
	clientSet, err := nodeutil.ClientsetFromEnv(config.KubeConfigPath)
	if err != nil {
		logrus.Errorf("Error creating client set: %v", err)
		return nil, errors.Wrap(err, "error creating client set")
	}

	if config.MqttClient == nil {
		return nil, errors.New("mqtt client cannot be nil")
	}

	if config.NodeID == "" {
		return nil, errors.New("node name cannot be empty")
	}

	// Set up the pod podProvider.
	var provider *podlet.BaseProvider
	var nodeProvider *VirtualKubeletNode
	cm, err := nodeutil.NewNode(
		config.NodeID,
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, node.NodeProvider, error) {
			nodeProvider = NewVirtualKubeletNode(model.BuildVirtualNodeConfig{
				NodeIP:    config.NodeIP,
				TechStack: config.TechStack,
				Version:   config.BizVersion,
				BizName:   config.BizName,
			})
			// initialize node spec on bootstrap
			provider = podlet.NewBaseProvider(cfg.Node.Namespace, config.NodeIP, config.NodeID, config.MqttClient, clientSet)

			err := nodeProvider.Register(context.Background(), cfg.Node)
			if err != nil {
				return nil, nil, err
			}
			return provider, nodeProvider, nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			cfg.InformerResyncPeriod = time.Minute
			cfg.NodeSpec.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.NodeSpec.Status.NodeInfo.OperatingSystem = "linux"
			cfg.DebugHTTP = true

			cfg.NumWorkers = 4
			return nil
		},
		nodeutil.WithClient(clientSet),
	)
	if err != nil {
		return nil, err
	}

	return &KouplelessNode{
		clientSet:          clientSet,
		mqttClient:         config.MqttClient,
		podProvider:        provider,
		nodeID:             config.NodeID,
		vnode:              nodeProvider,
		node:               cm,
		done:               make(chan struct{}),
		ready:              make(chan struct{}),
		BaseBizExitChan:    make(chan struct{}),
		BaseBizInfoChan:    make(chan []ark.ArkBizInfo, 5),
		BaseHealthInfoChan: make(chan ark.HealthData, 5),
	}, nil
}
