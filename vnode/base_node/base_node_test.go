package base_node

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/common/testutil/mqtt_client"
	"github.com/koupleless/virtual-kubelet/tunnel/mqtt_tunnel"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"
)

func init() {
	mqtt.DefaultMqttClientInitFunc = mqtt_client.NewMockMqttClient
}

func TestNewBaseNode_NoTunnel(t *testing.T) {
	_, err := NewBaseNode(&BuildBaseNodeConfig{})
	assert.NotNil(t, err)
}

func TestNewBaseNode_NoNodeID(t *testing.T) {
	_, err := NewBaseNode(&BuildBaseNodeConfig{
		Tunnel: &mqtt_tunnel.MqttTunnel{},
	})
	assert.NotNil(t, err)
}

func TestNewBaseNode_NoClient(t *testing.T) {
	_, err := NewBaseNode(&BuildBaseNodeConfig{
		Tunnel: &mqtt_tunnel.MqttTunnel{},
		BaseID: "test-node",
	})
	assert.NotNil(t, err)
}

func TestNewBaseNode(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()

	fakeFactory := informers.NewSharedInformerFactory(kubeClient, time.Minute)

	fakeInformer := fakeFactory.Core().V1().Pods()
	fakeLister := fakeInformer.Lister()

	baseNode, err := NewBaseNode(&BuildBaseNodeConfig{
		KubeClient:  kubeClient,
		PodLister:   fakeLister,
		PodInformer: fakeInformer,
		Tunnel:      &mqtt_tunnel.MqttTunnel{},
		BaseID:      "test-node",
		NodeIP:      "127.0.0.1",
		TechStack:   "java",
		NodeName:    "testMaster",
		NodeVersion: "1.0.0",
	})
	assert.NotNil(t, baseNode)
	assert.Nil(t, err)
}

func TestBaseNode_Data_flow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kubeClient := fake.NewSimpleClientset()

	fakeFactory := informers.NewSharedInformerFactory(kubeClient, time.Minute)

	fakeInformer := fakeFactory.Core().V1().Pods()
	fakeLister := fakeInformer.Lister()

	baseNode, err := NewBaseNode(&BuildBaseNodeConfig{
		KubeClient:  kubeClient,
		PodLister:   fakeLister,
		PodInformer: fakeInformer,
		Tunnel:      &mqtt_tunnel.MqttTunnel{},
		BaseID:      "test-node",
		NodeIP:      "127.0.0.1",
		TechStack:   "java",
		NodeName:    "testMaster",
		NodeVersion: "1.0.0",
	})
	assert.NotNil(t, baseNode)
	assert.Nil(t, err)

	go baseNode.listenAndSync(ctx)

	baseNode.BaseBizInfoChan <- []ark.ArkBizInfo{}

	baseNode.BaseHealthInfoChan <- ark.HealthData{}

}

func TestBaseNode_RunAndContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := &mqtt_tunnel.MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", nil, nil, nil, nil, nil)
	assert.NoError(t, err)
	kubeClient := fake.NewSimpleClientset()

	fakeFactory := informers.NewSharedInformerFactory(kubeClient, time.Minute)

	fakeInformer := fakeFactory.Core().V1().Pods()
	fakeLister := fakeInformer.Lister()

	baseNode, err := NewBaseNode(&BuildBaseNodeConfig{
		KubeClient:  kubeClient,
		PodLister:   fakeLister,
		PodInformer: fakeInformer,
		Tunnel:      mt,
		BaseID:      "test-node",
		NodeIP:      "127.0.0.1",
		TechStack:   "java",
		NodeName:    "testMaster",
		NodeVersion: "1.0.0",
	})
	assert.NotNil(t, baseNode)
	assert.Nil(t, err)
	go baseNode.Run(ctx)
	time.Sleep(1 * time.Second)
	cancel()
	<-baseNode.Done()
	assert.NotNil(t, baseNode.Err())
}

func TestBaseNode_RunAndExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := &mqtt_tunnel.MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", nil, nil, nil, nil, nil)
	assert.NoError(t, err)
	kubeClient := fake.NewSimpleClientset()

	fakeFactory := informers.NewSharedInformerFactory(kubeClient, time.Minute)

	fakeInformer := fakeFactory.Core().V1().Pods()
	fakeLister := fakeInformer.Lister()

	baseNode, err := NewBaseNode(&BuildBaseNodeConfig{
		KubeClient:  kubeClient,
		PodLister:   fakeLister,
		PodInformer: fakeInformer,
		Tunnel:      mt,
		BaseID:      "test-node",
		NodeIP:      "127.0.0.1",
		TechStack:   "java",
		NodeName:    "testMaster",
		NodeVersion: "1.0.0",
	})
	assert.NotNil(t, baseNode)
	assert.Nil(t, err)
	go baseNode.Run(ctx)
	err = baseNode.WaitReady(ctx, time.Minute)
	assert.Nil(t, err)
	time.Sleep(1 * time.Second)
	baseNode.Exit()
	<-baseNode.Done()
	assert.Nil(t, baseNode.Err())
}

func TestBaseNode_PodOperator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := &mqtt_tunnel.MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", nil, nil, nil, nil, nil)
	assert.NoError(t, err)
	kubeClient := fake.NewSimpleClientset()

	fakeFactory := informers.NewSharedInformerFactory(kubeClient, time.Minute)

	fakeInformer := fakeFactory.Core().V1().Pods()
	fakeLister := fakeInformer.Lister()

	baseNode, err := NewBaseNode(&BuildBaseNodeConfig{
		KubeClient:  kubeClient,
		PodLister:   fakeLister,
		PodInformer: fakeInformer,
		Tunnel:      mt,
		BaseID:      "test-node",
		NodeIP:      "127.0.0.1",
		TechStack:   "java",
		NodeName:    "testMaster",
		NodeVersion: "1.0.0",
	})
	assert.NotNil(t, baseNode)
	assert.Nil(t, err)
	go baseNode.Run(ctx)
	err = baseNode.WaitReady(ctx, time.Minute)
	assert.Nil(t, err)
	time.Sleep(1 * time.Second)

	podKey := "test-pod"
	srcPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}
	baseNode.PodStore(podKey, &srcPod)

	baseNode.SyncPodsFromKubernetesEnqueue(ctx, podKey)

	pod, has := baseNode.LoadPodFromController(podKey)
	assert.True(t, has)
	assert.NotNil(t, pod)

	podCopy := srcPod.DeepCopy()
	podCopy.Name = "test-pod2"
	baseNode.CheckAndUpdatePod(ctx, podKey, pod, podCopy)

	pod, has = baseNode.LoadPodFromController(podKey)
	assert.True(t, has)
	assert.NotNil(t, pod)

	baseNode.DeletePod(podKey)

	pod, has = baseNode.LoadPodFromController(podKey)
	assert.False(t, has)
	assert.Nil(t, pod)

	baseNode.DeletePodsFromKubernetesForget(ctx, podKey)

	baseNode.Exit()
	<-baseNode.Done()
	assert.Nil(t, baseNode.Err())
}
