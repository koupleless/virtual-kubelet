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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	appv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/homedir"
	"os"
	"path"
	"testing"
	"time"
)

var (
	buildVersion = "N/A"
	k8sVersion   = "v1.15.2" // This should follow the version of k8s.io/kubernetes we are importing
)

const (
	DefaultNamespace = metav1.NamespaceDefault
)

var k8sClient kubernetes.Interface

var basePodDeployment *appv1.Deployment
var err error
var DefaultKubeConfigPath = path.Join(homedir.HomeDir(), ".kube", "config")

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

func TestVirtualKubelet(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Virtual Kubelet Suite")
}

var _ = BeforeSuite(func() {
	By("preparing test environment")
	k8sClient, err = nodeutil.ClientsetFromEnv(DefaultKubeConfigPath)
	Expect(err).NotTo(HaveOccurred())
	startBasePodDeployment()
	time.Sleep(time.Second * 5)
})

var _ = AfterSuite(func() {
	By("shutting down test environment")
	if basePodDeployment != nil {
		shutdownBasePodDeployment(basePodDeployment.Name)
	}
})

func startBasePodDeployment() {
	basePodDeploymentTemplate := getBasePodTemplate()
	newDeployment, err := k8sClient.AppsV1().Deployments(DefaultNamespace).Create(context.Background(), basePodDeploymentTemplate, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
	Expect(newDeployment).NotTo(BeNil())
	basePodDeployment = newDeployment

	Eventually(func() bool {
		// wait for base pod ready
		newDeployment, err = k8sClient.AppsV1().Deployments(DefaultNamespace).Get(context.Background(), basePodDeploymentTemplate.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(newDeployment).NotTo(BeNil())
		basePodDeployment = newDeployment
		return newDeployment.Status.ReadyReplicas == newDeployment.Status.Replicas
	}, time.Minute*2, time.Second).Should(BeTrue())
}

func shutdownBasePodDeployment(name string) {
	err = k8sClient.AppsV1().Deployments(DefaultNamespace).Delete(context.Background(), name, metav1.DeleteOptions{})
	Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	Eventually(func() bool {
		err = k8sClient.AppsV1().Deployments(DefaultNamespace).Delete(context.Background(), name, metav1.DeleteOptions{})
		return errors.IsNotFound(err)
	}, time.Minute, time.Second).Should(BeTrue())
}

func getBasePodTemplate() *appv1.Deployment {
	var deployment appv1.Deployment
	basePodYamlFilePath := path.Join("../samples", "base_pod_deployment.yaml")
	content, err := os.ReadFile(basePodYamlFilePath)
	Expect(err).NotTo(HaveOccurred())
	err = yaml.Unmarshal(content, &deployment)
	Expect(err).NotTo(HaveOccurred())
	return &deployment
}

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
