package test

import (
	"context"
	base_pod "github.com/koupleless/virtual-kubelet/java/pod/base_pod_controller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path"
	"time"
)

// complete base pod manage suite test
var _ = Describe("Base Pod Management", func() {

	const timeout = time.Second * 60

	const interval = time.Second * 3

	ctx := context.WithValue(context.Background(), "env", "base-pod-management")

	singleModuleBiz1PodYamlFilePath := path.Join("../samples", "single_module_biz1_to_mock_base.yaml")
	singleModuleBiz2PodYamlFilePath := path.Join("../samples", "single_module_biz2_to_mock_base.yaml")

	singleModuleBiz1Pod, err := getPodFromYamlFile(singleModuleBiz1PodYamlFilePath)
	It("should load biz1 yaml successfully", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(singleModuleBiz1Pod).NotTo(BeNil())
	})

	singleModuleBiz2Pod, err := getPodFromYamlFile(singleModuleBiz2PodYamlFilePath)
	It("should load biz2 yaml successfully", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(singleModuleBiz2Pod).NotTo(BeNil())
	})

	Context("base pod finalizers management", func() {
		It("should mock base pod start successfully", func() {
			// start a test base pod
			startBasePodDeployment(MockBasePodName, MockBasePodVersion, MockVNodeListPort, func(pod *corev1.Pod) {
				mockBasePod = pod
			})
		})

		It("should contains finalizer after first module running", func() {
			// publish module pod
			_, err = k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, singleModuleBiz1Pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(time.Second)
			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				return pod.Status.Phase == corev1.PodRunning
			}, timeout, interval).Should(BeTrue())

			// check base pod, should contain finalizers with module name
			currBasePod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, mockBasePod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(currBasePod).NotTo(BeNil())
			Expect(currBasePod.Finalizers).To(HaveLen(1))
			Expect(currBasePod.Finalizers[0]).To(Equal(base_pod.FormatModuleFinalizerKey(singleModuleBiz1Pod.Name)))
		})

		It("should contains finalizer after second module running", func() {
			// publish module pod
			_, err = k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, singleModuleBiz2Pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz2Pod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				return pod.Status.Phase == corev1.PodRunning
			}, timeout, interval).Should(BeTrue())

			// check base pod, should contain finalizers with module name
			currBasePod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, mockBasePod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(currBasePod).NotTo(BeNil())
			Expect(base_pod.FormatModuleFinalizerKey(singleModuleBiz2Pod.Name)).To(BeElementOf(currBasePod.Finalizers))
			Expect(base_pod.FormatModuleFinalizerKey(singleModuleBiz1Pod.Name)).To(BeElementOf(currBasePod.Finalizers))
		})

		It("should not contains finalizer after first module deleting", func() {
			// delete module pod
			err = k8sClient.CoreV1().Pods(DefaultNamespace).Delete(ctx, singleModuleBiz1Pod.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			// waiting for pod deletion complete
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// check base pod, should contain finalizers with module name
			currBasePod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, mockBasePod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(currBasePod).NotTo(BeNil())
			Expect(base_pod.FormatModuleFinalizerKey(singleModuleBiz1Pod.Name)).NotTo(BeElementOf(currBasePod.Finalizers))
			Expect(base_pod.FormatModuleFinalizerKey(singleModuleBiz2Pod.Name)).To(BeElementOf(currBasePod.Finalizers))
		})

		It("should not contains finalizer after second module deleting", func() {
			// delete module pod
			err = k8sClient.CoreV1().Pods(DefaultNamespace).Delete(ctx, singleModuleBiz2Pod.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			// waiting for pod deletion complete
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz2Pod.Name, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// check base pod, should contain finalizers with module name
			currBasePod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, mockBasePod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(currBasePod).NotTo(BeNil())
			Expect(base_pod.FormatModuleFinalizerKey(singleModuleBiz2Pod.Name)).NotTo(BeElementOf(currBasePod.Finalizers))
		})
	})

	Context("base pod deletion", func() {
		It("should delete vnode when deletion start", func() {
			// publish module pod
			_, err = k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, singleModuleBiz1Pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				return pod.Status.Phase == corev1.PodRunning
			}, timeout, interval).Should(BeTrue())

			// delete basic base pod
			err = k8sClient.CoreV1().Pods(DefaultNamespace).Delete(ctx, mockBasePod.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			// check vnode
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Nodes().Get(ctx, mockBasePod.Name, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// check vpod
			Eventually(func() bool {
				_, basicPodGetErr := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, mockBasePod.Name, metav1.GetOptions{})
				_, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				if err == nil {
					// basic pod should not being deleted before all of vpod deleted
					Expect(basicPodGetErr).NotTo(HaveOccurred())
				}
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// after vpod being evicted, base pod should being deleted
			Eventually(func() bool {
				_, err = k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, mockBasePod.Name, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
