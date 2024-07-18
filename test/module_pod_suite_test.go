package test

import (
	"context"
	"github.com/koupleless/virtual-kubelet/java/pod/node"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path"
	"time"
)

var _ = Describe("Module Publish", func() {

	const timeout = time.Second * 60

	const interval = time.Second * 3

	ctx := context.WithValue(context.Background(), "env", "module-publish-test")
	singleModuleBiz1PodYamlFilePath := path.Join("../samples", "single_module_biz1.yaml")
	singleModuleBiz1Version2PodYamlFilePath := path.Join("../samples", "single_module_biz1_version2.yaml")
	multiModulePodYamlFilePath := path.Join("../samples", "multi_module_biz1_biz2.yaml")

	singleModuleBiz1Pod, err := getPodFromYamlFile(singleModuleBiz1PodYamlFilePath)

	It("should init successfully", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(singleModuleBiz1Pod).NotTo(BeNil())
	})

	var mockBase *BaseMock
	nodeId := "test-base"

	It("mock base should start successfully", func() {
		mockBase = NewBaseMock(nodeId, "base", "1.1.1", baseMqttClient)
		go mockBase.Run()
		Eventually(func() bool {
			_, err := k8sClient.CoreV1().Nodes().Get(ctx, node.VIRTUAL_NODE_NAME_PREFIX+nodeId, metav1.GetOptions{})
			return !errors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	Context("create single module pod", func() {
		It("should create successfully", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, singleModuleBiz1Pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
		})

		It("should change to running status finally", func() {
			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				logrus.Infof("pod status: %v", pod.Status)
				return pod.Status.Phase == corev1.PodRunning
			}, timeout, interval).Should(BeTrue())
		})

		It("should have right status", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
			Expect(pod.Status.Conditions).To(HaveLen(4))
			for _, cond := range pod.Status.Conditions {
				Expect(cond.Status).To(Equal(corev1.ConditionTrue))
			}
		})
	})

	Context("delete single module pod", func() {
		It("should be terminating status", func() {
			Expect(k8sClient.CoreV1().Pods(DefaultNamespace).Delete(ctx, singleModuleBiz1Pod.Name, metav1.DeleteOptions{})).NotTo(HaveOccurred())
		})

		It("should delete finally", func() {
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	multiModulePod, err := getPodFromYamlFile(multiModulePodYamlFilePath)

	Context("create multi module pod", func() {
		It("should create successfully", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(multiModulePod).NotTo(BeNil())
			_, err = k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, multiModulePod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be pending status", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, multiModulePod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
			Expect(pod.Status.Phase).To(Equal(corev1.PodPending))
		})

		It("should change to running status finally", func() {
			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, multiModulePod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				logrus.Infof("pod status: %v", pod.Status)
				return pod.Status.Phase == corev1.PodRunning
			}, timeout, interval).Should(BeTrue())
		})

		It("should have right status", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, multiModulePod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
			Expect(pod.Status.Conditions).To(HaveLen(4))
			for _, cond := range pod.Status.Conditions {
				Expect(cond.Status).To(Equal(corev1.ConditionTrue))
			}
		})
	})

	Context("delete multi module pod", func() {
		It("should be terminating status", func() {
			Expect(k8sClient.CoreV1().Pods(DefaultNamespace).Delete(ctx, multiModulePod.Name, metav1.DeleteOptions{})).NotTo(HaveOccurred())
		})

		It("should delete finally", func() {
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, multiModulePod.Name, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	singleModuleBiz1Version2, err := getPodFromYamlFile(singleModuleBiz1Version2PodYamlFilePath)

	It("should init successfully", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(singleModuleBiz1Version2).NotTo(BeNil())
	})

	Context("module version update", func() {
		It("should biz1 version1 create successfully", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, singleModuleBiz1Pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
		})

		It("should biz1 version1 change to running status finally", func() {
			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				logrus.Infof("pod status: %v", pod.Status)
				return pod.Status.Phase == corev1.PodRunning
			}, timeout, interval).Should(BeTrue())
		})

		It("should biz1 version2 create successfully", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, singleModuleBiz1Version2, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
		})

		It("should biz2 version2 change to running status finally", func() {
			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Version2.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				logrus.Infof("pod status: %v", pod.Status)
				return pod.Status.Phase == corev1.PodRunning
			}, timeout, interval).Should(BeTrue())
		})

		It("should biz1 version1 change to Failed status finally", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
			logrus.Infof("pod status: %v", pod.Status)
			Expect(pod.Status.Phase).To(Equal(corev1.PodFailed))
		})

		It("should all module delete finally", func() {
			err := k8sClient.CoreV1().Pods(DefaultNamespace).Delete(ctx, singleModuleBiz1Pod.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.CoreV1().Pods(DefaultNamespace).Delete(ctx, singleModuleBiz1Version2.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				_, errBiz1V1 := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				_, errBiz1V2 := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Version2.Name, metav1.GetOptions{})
				return errors.IsNotFound(errBiz1V1) && errors.IsNotFound(errBiz1V2)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("base exit", func() {
		It("publish a module and should be running status finally", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, singleModuleBiz1Pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				logrus.Infof("pod status: %v", pod.Status)
				return pod.Status.Phase == corev1.PodRunning
			}, timeout, interval).Should(BeTrue())
		})

		It("base exit", func() {
			if mockBase != nil {
				mockBase.Exit()
			}
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Nodes().Get(ctx, node.VIRTUAL_NODE_NAME_PREFIX+nodeId, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("pod should be deleted", func() {
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
