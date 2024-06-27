package test

import (
	"context"
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
	singleModuleBiz1PodYamlFilePath := path.Join("samples", "single_module_biz1_to_basic_base.yaml")
	multiModulePodYamlFilePath := path.Join("samples", "multi_module_biz1_biz2.yaml")

	singleModuleBiz1Pod, err := getPodFromYamlFile(singleModuleBiz1PodYamlFilePath)

	Context("create single module pod", func() {
		It("should create successfully", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(singleModuleBiz1Pod).NotTo(BeNil())
			_, err = k8sClient.CoreV1().Pods(DefaultNamespace).Create(ctx, singleModuleBiz1Pod, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should be pending status", func() {
			pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).NotTo(BeNil())
			Expect(pod.Status.Phase).To(Equal(corev1.PodPending))
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

			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, singleModuleBiz1Pod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				logrus.Infof("pod status: %v", pod.Status)
				return pod.Status.Phase == corev1.PodSucceeded
			}, timeout, interval).Should(BeTrue())
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

			Eventually(func() bool {
				pod, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, multiModulePod.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(pod).NotTo(BeNil())
				logrus.Infof("pod status: %v", pod.Status)
				return pod.Status.Phase == corev1.PodSucceeded
			}, timeout, interval).Should(BeTrue())
		})

		It("should delete finally", func() {
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Pods(DefaultNamespace).Get(ctx, multiModulePod.Name, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
