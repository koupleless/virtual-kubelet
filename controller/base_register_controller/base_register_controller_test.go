package base_register_controller

import (
	"context"
	"fmt"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/common/testutil/base"
	"github.com/koupleless/virtual-kubelet/common/testutil/mqtt_broker"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/tunnel/mqtt_tunnel"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"
)

func TestNewBaseRegisterController_ConfigNil(t *testing.T) {
	controller, err := NewBaseRegisterController(nil)
	assert.Error(t, err)
	assert.Nil(t, controller)
}

func TestNewBaseRegisterController_TunnelNotProvided(t *testing.T) {
	controller, err := NewBaseRegisterController(nil)
	assert.Error(t, err)
	assert.Nil(t, controller)
}

func TestNewBaseRegisterController(t *testing.T) {
	go mqtt_broker.StartLocalMqttBroker()
	kubeClient := fake.NewSimpleClientset()

	controller, err := NewBaseRegisterController(&BuildBaseRegisterControllerConfig{
		ClientID: "test-client",
		K8SConfig: &K8SConfig{
			KubeClient:         kubeClient,
			InformerSyncPeriod: time.Minute,
		},
		Tunnels: []tunnel.Tunnel{
			&mqtt_tunnel.MqttTunnel{},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, controller)
}

func TestBaseRegisterController_Run(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mqtt_broker.StartLocalMqttBroker()
	kubeClient := fake.NewSimpleClientset()

	controller, err := NewBaseRegisterController(&BuildBaseRegisterControllerConfig{
		ClientID: "test-client",
		K8SConfig: &K8SConfig{
			KubeClient:         kubeClient,
			InformerSyncPeriod: time.Minute,
		},
		Tunnels: []tunnel.Tunnel{
			&mqtt_tunnel.MqttTunnel{},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, controller)

	go controller.Run(ctx)
	<-controller.Ready()

	// mock base online
	client, err := mqtt.NewMqttClient(&mqtt.ClientConfig{
		Broker:   "localhost",
		Port:     1883,
		ClientID: "TestNewMqttClientID",
		Username: "local",
		Password: "public",
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	// mock base online
	id := "test-base"
	mockBase := base.NewBaseMock(id, "base", "1.0.0", client)

	// add pod, update pod and delete pod
	controller.podAddHandler(nil)

	srcPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: model.VIRTUAL_NODE_NAME_PREFIX + id,
			Containers: []corev1.Container{
				{
					Name:  "test-biz",
					Image: "test-biz-url",
					Env: []corev1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "1.0.0",
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{},
	}
	podCopy := srcPod.DeepCopy()
	podCopy.Spec.Containers = []corev1.Container{
		{
			Name:  "test-biz",
			Image: "test-biz-url",
			Env: []corev1.EnvVar{
				{
					Name:  "BIZ_VERSION",
					Value: "1.0.1",
				},
			},
		},
	}
	controller.podUpdateHandler(srcPod, podCopy)
	controller.podDeleteHandler(srcPod)
	controller.podAddHandler(srcPod)

	go mockBase.Run()

	assert.Eventually(t, func() bool {
		return len(controller.localStore.baseIDToBaseNode) == 1
	}, time.Second*5, time.Millisecond*200)

	controller.podAddHandler(srcPod)

	controller.podUpdateHandler(srcPod, podCopy)

	controller.podDeleteHandler(podCopy)

	mockBase.Exit()
	controller.localStore.baseLatestMsgTime["test-offline"] = 0
	controller.checkAndDeleteOfflineBase(ctx)

	cancel()
	<-controller.Done()
}
