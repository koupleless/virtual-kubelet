package node

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/java/common"
	"github.com/koupleless/virtual-kubelet/java/model"
	podlet "github.com/koupleless/virtual-kubelet/java/pod/let"
	"github.com/koupleless/virtual-kubelet/node"
	"github.com/koupleless/virtual-kubelet/node/nodeutil"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
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

	done               chan struct{}
	BaseHealthInfoChan chan ark.HealthData
	BaseBizInfoChan    chan []ark.ArkBizInfo
	BaseBizExitChan    chan struct{}

	err error
}

func (n *KouplelessNode) Run(ctx context.Context) {
	var err error

	// process vkNode run and bpc run, catching error
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
		n.config.MqttClient.Pub(common.FormatArkletCommandTopic(n.config.NodeID, model.CommandHealth), mqtt.Qos0, "{}")
	})

	go common.TimedTaskWithInterval(ctx, time.Second*5, func(ctx context.Context) {
		n.config.MqttClient.Pub(common.FormatArkletCommandTopic(n.config.NodeID, model.CommandQueryAllBiz), mqtt.Qos0, "{}")
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

func (n *KouplelessNode) WaitReady(ctx context.Context, timeout time.Duration) error {
	return n.node.WaitReady(ctx, timeout)
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

func (n *KouplelessNode) PodStore(key string, pod *corev1.Pod) {
	n.node.PodController().StorePod(key, pod)
}

func (n *KouplelessNode) LoadPodFromController(key string) (any, bool) {
	return n.node.PodController().LoadPod(key)
}

func (n *KouplelessNode) CheckAndUpdatePod(ctx context.Context, key string, obj interface{}, pod *corev1.Pod) {
	n.node.PodController().CheckAndUpdatePod(ctx, key, obj, pod)
}

func (n *KouplelessNode) SyncPodsFromKubernetesEnqueue(ctx context.Context, key string) {
	n.node.PodController().SyncPodsFromKubernetesEnqueue(ctx, key)
}

func (n *KouplelessNode) DeletePodsFromKubernetesForget(ctx context.Context, key string) {
	n.node.PodController().DeletePodsFromKubernetesForget(ctx, key)
}

func (n *KouplelessNode) DeletePod(key string) {
	n.node.PodController().DeletePod(key)
}

func NewKouplelessNode(config *model.BuildKouplelessNodeConfig) (kn *KouplelessNode, err error) {
	if config.MqttClient == nil {
		return nil, errors.New("mqtt client cannot be nil")
	}

	if config.NodeID == "" {
		return nil, errors.New("node name cannot be empty")
	}
	var vnode *VirtualKubeletNode
	var podProvider *podlet.BaseProvider
	cm, err := nodeutil.NewNode(
		VIRTUAL_NODE_NAME_PREFIX+config.NodeID,
		func(cfg nodeutil.ProviderConfig) (nodeutil.Provider, node.NodeProvider, error) {
			vnode = NewVirtualKubeletNode(model.BuildVirtualNodeConfig{
				NodeIP:    config.NodeIP,
				TechStack: config.TechStack,
				Version:   config.BizVersion,
				BizName:   config.BizName,
			})
			// initialize node spec on bootstrap
			podProvider = podlet.NewBaseProvider(cfg.Node.Namespace, config.NodeIP, config.NodeID, config.MqttClient, config.KubeClient)

			err = vnode.Register(context.Background(), cfg.Node)
			if err != nil {
				return nil, nil, err
			}
			return podProvider, vnode, nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			cfg.NodeSpec.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.NodeSpec.Status.NodeInfo.OperatingSystem = "linux"

			cfg.NumWorkers = 1
			return nil
		},
		nodeutil.WithClient(config.KubeClient),
		nodeutil.WithPodLister(config.PodLister),
	)
	if err != nil {
		return nil, err
	}

	return &KouplelessNode{
		config:             *config,
		clientSet:          config.KubeClient,
		vnode:              vnode,
		podProvider:        podProvider,
		node:               cm,
		done:               make(chan struct{}),
		BaseBizInfoChan:    make(chan []ark.ArkBizInfo),
		BaseHealthInfoChan: make(chan ark.HealthData),
		BaseBizExitChan:    make(chan struct{}),
	}, nil
}
