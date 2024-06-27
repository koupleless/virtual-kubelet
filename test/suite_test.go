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
	"fmt"
	"github.com/koupleless/virtual-kubelet/commands/root"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/homedir"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

var (
	buildVersion = "N/A"
	k8sVersion   = "v1.15.2" // This should follow the version of k8s.io/kubernetes we are importing
)

const (
	DefaultNamespace    = metav1.NamespaceDefault
	BasicBasePodName    = "test-base-pod-basic"
	BasicBasePodVersion = "1.1.1"
	BasicVNodeListPort  = 10250
	MockBasePodName     = "test-base-pod-mock"
	MockBasePodVersion  = "1.1.2"
	MockVNodeListPort   = 10251
)

var k8sClient kubernetes.Interface

var basicBasePod *corev1.Pod
var mockBasePod *corev1.Pod
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
	startBasePod(BasicBasePodName, BasicBasePodVersion, BasicVNodeListPort, func(pod *corev1.Pod) {
		basicBasePod = pod
	})
	time.Sleep(time.Second * 5)
})

var _ = AfterSuite(func() {
	By("shutting down test environment")
	shutdownBasePod(BasicBasePodName)
	shutdownBasePod(MockBasePodName)
})

func startBasePod(basePodName, baseVersion string, listenPort int32, cb func(*corev1.Pod)) {
	// deploy mock base pod
	initBasePod(basePodName, cb)

	initBasicEnvWithBasePod(basePodName, baseVersion)

	ctx := context.WithValue(context.Background(), "env", "suite_test_environment")

	var opts root.Opts
	optsErr := root.SetDefaultOpts(&opts)
	opts.Version = strings.Join([]string{k8sVersion, "vk", buildVersion}, "-")
	opts.ListenPort = listenPort

	rootCmd := root.NewCommand(ctx, opts)
	preRun := rootCmd.PreRunE

	rootCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if optsErr != nil {
			return optsErr
		}
		if preRun != nil {
			return preRun(cmd, args)
		}
		return nil
	}

	go func() {
		err = rootCmd.Execute()
		if err != nil {
			fmt.Println(err)
		}
		Expect(err).NotTo(HaveOccurred())
	}()
}

func shutdownBasePod(basePodName string) {
	err = k8sClient.CoreV1().Pods(DefaultNamespace).Delete(context.Background(), basePodName, metav1.DeleteOptions{})
	Expect(err == nil || errors.IsNotFound(err)).To(BeTrue())

	Eventually(func() bool {
		_, err = k8sClient.CoreV1().Pods(DefaultNamespace).Get(context.Background(), basePodName, metav1.GetOptions{})
		return errors.IsNotFound(err)
	}, time.Minute, time.Second).Should(BeTrue())
}

func initBasePod(name string, cb func(*corev1.Pod)) {
	basePodTemplate := getBasePodTemplate(name)
	newPod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Create(context.Background(), basePodTemplate, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
	Expect(newPod).NotTo(BeNil())
	cb(newPod)

	Eventually(func() bool {
		// wait for base pod ready
		newPod, err = k8sClient.CoreV1().Pods(DefaultNamespace).Get(context.Background(), name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(newPod).NotTo(BeNil())
		cb(newPod)
		return newPod.Status.Phase == corev1.PodRunning
	}, time.Minute, time.Second).Should(BeTrue())
}

func initBasicEnvWithBasePod(podName, baseVersion string) {
	var pod *corev1.Pod
	switch podName {
	case BasicBasePodName:
		pod = basicBasePod
	case MockBasePodName:
		pod = mockBasePod
	}
	Expect(pod).NotTo(BeNil())
	err := os.Setenv("BASE_POD_NAME", pod.Name)
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("BASE_POD_NAMESPACE", pod.Namespace)
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("BASE_POD_IP", pod.Status.PodIP)
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("TECH_STACK", "java")
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("VNODE_NAME", pod.Name)
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("VNODE_POD_CAPACITY", "5")
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("VNODE_VERSION", baseVersion)
	Expect(err).NotTo(HaveOccurred())

	err = os.Setenv("KUBE_NAME_SPACE", pod.Namespace)
	Expect(err).NotTo(HaveOccurred())

	// just for local test, online will use in-cluster kube config
	err = os.Setenv("BASE_POD_KUBE_CONFIG_PATH", DefaultKubeConfigPath)
	Expect(err).NotTo(HaveOccurred())
}

func getBasePodTemplate(name string) *corev1.Pod {
	var pod corev1.Pod
	basePodYamlFilePath := path.Join("samples", "base_pod_config.yaml")
	content, err := os.ReadFile(basePodYamlFilePath)
	Expect(err).NotTo(HaveOccurred())
	err = yaml.Unmarshal(content, &pod)
	Expect(err).NotTo(HaveOccurred())
	pod.Name = name
	return &pod
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
