package test

/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/controller/base_register_controller"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/homedir"
	"log"
	"os"
	"path"
	"testing"
	"time"
)

const (
	DefaultNamespace = metav1.NamespaceDefault
)

var k8sClient kubernetes.Interface
var baseMqttClient *mqtt.Client

var err error
var DefaultKubeConfigPath = path.Join(homedir.HomeDir(), ".kube", "config")

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

func TestVirtualKubelet(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Virtual Kubelet Suite")
}

var mainContext context.Context
var mainCancel context.CancelFunc

var _ = BeforeSuite(func() {
	mainContext, mainCancel = context.WithCancel(context.Background())
	By("preparing test environment")
	k8sClient, err = nodeutil.ClientsetFromEnv(DefaultKubeConfigPath)
	Expect(err).NotTo(HaveOccurred())
	baseMqttClient, err = mqtt.NewMqttClient(&mqtt.ClientConfig{
		Broker:   "broker.emqx.io",
		Port:     1883,
		ClientID: "base-mqtt-client",
		Username: "emqx",
		Password: "public",
	})
	Expect(err).NotTo(HaveOccurred())
	// start mc
	registerController, err := base_register_controller.NewBaseRegisterController(&base_register_controller.BuildBaseRegisterControllerConfig{
		K8SConfig: &base_register_controller.K8SConfig{
			KubeConfigPath:     DefaultKubeConfigPath,
			InformerSyncPeriod: time.Minute * 5,
		},
		// TODO add provider
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(registerController).NotTo(BeNil())

	go registerController.Run(mainContext)

	<-registerController.Ready()
	log.Println("Registering controller ready")
})

var _ = AfterSuite(func() {
	By("shutting down test environment")
	mainCancel()
})

func getPodFromYamlFile(filePath string) (*corev1.Pod, error) {
	var pod corev1.Pod
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(content, &pod)
	if err != nil {
		return nil, err
	}
	return &pod, nil
}

func getDeploymentFromYamlFile(filePath string) (*v1.Deployment, error) {
	var deployment v1.Deployment
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(content, &deployment)
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

func getDaemonSetFromYamlFile(filePath string) (*v1.DaemonSet, error) {
	var daemonSet v1.DaemonSet
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(content, &daemonSet)
	if err != nil {
		return nil, err
	}
	return &daemonSet, nil
}
