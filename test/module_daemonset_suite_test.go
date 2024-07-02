package test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"path"
	"time"
)

// use daemon set to achieve module peer deployment
var _ = Describe("Module DaemonSet", func() {

	const timeout = time.Second * 90

	const interval = time.Second * 3

	var err error

	ctx := context.WithValue(context.Background(), "env", "module-daemonSet")

	moduleDaemonSetYamlFilePath := path.Join("../samples", "module_daemonset.yaml")

	moduleDaemonSet, err := getDaemonSetFromYamlFile(moduleDaemonSetYamlFilePath)
	It("yaml should load successfully", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(moduleDaemonSet).NotTo(BeNil())
	})

	var selector labels.Selector
	It("should init selector successfully", func() {
		requirement, err := labels.NewRequirement("app", selection.Equals, []string{moduleDaemonSet.Name})
		Expect(err).ToNot(HaveOccurred())
		selector = labels.NewSelector().Add(*requirement)
	})

	Context("create daemonSet", func() {
		It("should create successfully", func() {
			createResult, err := k8sClient.AppsV1().DaemonSets(DefaultNamespace).Create(ctx, moduleDaemonSet, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(createResult).NotTo(BeNil())

			moduleDaemonSet = createResult
		})

		It("should all of the pod become running finally and the num of pods should be 1", func() {
			Eventually(func() bool {
				podList, err := k8sClient.CoreV1().Pods(DefaultNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: selector.String(),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(podList).NotTo(BeNil())
				isAllReady := true
				for _, pod := range podList.Items {
					isAllReady = isAllReady && pod.Status.Phase == corev1.PodRunning
				}
				return isAllReady && len(podList.Items) == 1
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("base pod scale", func() {
		It("should base pod scale successfully", func() {
			currScale, err := k8sClient.AppsV1().Deployments(DefaultNamespace).GetScale(ctx, basePodDeployment.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(currScale).NotTo(BeNil())
			Expect(currScale.Spec.Replicas).To(Equal(int32(1)))

			// scale up to 3 replicas
			currScale.Spec.Replicas += 2
			scaleResult, err := k8sClient.AppsV1().Deployments(DefaultNamespace).UpdateScale(ctx, basePodDeployment.Name, currScale, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(scaleResult).NotTo(BeNil())
		})

		It("should all of the pod become running finally and the num of pods should be 2", func() {
			Eventually(func() bool {
				podList, err := k8sClient.CoreV1().Pods(DefaultNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: selector.String(),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(podList).NotTo(BeNil())
				isAllReady := true
				for _, pod := range podList.Items {
					isAllReady = isAllReady && pod.Status.Phase == corev1.PodRunning
				}
				return isAllReady && len(podList.Items) == 3
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("update daemon set", func() {
		It("should update successfully", func() {
			// get current deployment info
			moduleDaemonSet, err = k8sClient.AppsV1().DaemonSets(DefaultNamespace).Get(context.Background(), moduleDaemonSet.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(moduleDaemonSet).NotTo(BeNil())
			// publish new template
			moduleDaemonSet.Spec.Template.Spec.Containers[0].Name = "biz2"
			moduleDaemonSet.Spec.Template.Spec.Containers[0].Image = "https://serverless-opensource.oss-cn-shanghai.aliyuncs.com/module-packages/stable/biz2-web-single-host-0.0.1-SNAPSHOT-ark-biz.jar"
			updatedDaemonSet, err := k8sClient.AppsV1().DaemonSets(DefaultNamespace).Update(ctx, moduleDaemonSet, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedDaemonSet).NotTo(BeNil())
			Expect(len(updatedDaemonSet.Spec.Template.Spec.Containers)).To(Equal(1))
			Expect(updatedDaemonSet.Spec.Template.Spec.Containers[0].Name).To(Equal("biz2"))
		})

		It("should have correct revisions", func() {
			controllerRevisionRequirement, err := labels.NewRequirement("app", selection.Equals, []string{moduleDaemonSet.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerRevisionRequirement).NotTo(BeNil())
			controllerRevisionSelector := labels.NewSelector().Add(*controllerRevisionRequirement)
			controllerRevisionList, err := k8sClient.AppsV1().ControllerRevisions(DefaultNamespace).List(ctx, metav1.ListOptions{LabelSelector: controllerRevisionSelector.String()})
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerRevisionList).NotTo(BeNil())
			// a new and the old one
			Expect(len(controllerRevisionList.Items)).To(Equal(2))
		})

		It("should all of the pod become new pod finally", func() {
			Eventually(func() bool {
				podList, err := k8sClient.CoreV1().Pods(DefaultNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: selector.String(),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(podList).NotTo(BeNil())
				numOfPodContainerName := make(map[string]int)
				for _, pod := range podList.Items {
					for _, container := range pod.Spec.Containers {
						numOfPodContainerName[container.Name] += 1
					}
				}
				isAllReady := true
				for _, pod := range podList.Items {
					isAllReady = isAllReady && pod.Status.Phase == corev1.PodRunning
				}
				// only contains biz2 module
				return isAllReady && len(podList.Items) == 3 && len(numOfPodContainerName) == 1 && numOfPodContainerName["biz2"] != 0
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("base pod deployment scale down", func() {
		It("should base pod deployment scale to 1 successfully", func() {
			currScale, err := k8sClient.AppsV1().Deployments(DefaultNamespace).GetScale(ctx, basePodDeployment.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(currScale).NotTo(BeNil())
			Expect(currScale.Spec.Replicas).To(Equal(int32(3)))

			// scale down to 3 replicas
			currScale.Spec.Replicas = 1
			scaleResult, err := k8sClient.AppsV1().Deployments(DefaultNamespace).UpdateScale(ctx, basePodDeployment.Name, currScale, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(scaleResult).NotTo(BeNil())
		})

		It("should not create new pod", func() {
			Eventually(func() bool {
				podList, err := k8sClient.CoreV1().Pods(DefaultNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: selector.String(),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(podList).NotTo(BeNil())
				return len(podList.Items) == 1
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("delete daemon set", func() {
		It("should all of the pods be terminating status", func() {
			Expect(k8sClient.AppsV1().DaemonSets(DefaultNamespace).Delete(ctx, moduleDaemonSet.Name, metav1.DeleteOptions{})).NotTo(HaveOccurred())

			Eventually(func() bool {
				podList, err := k8sClient.CoreV1().Pods(DefaultNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: selector.String(),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(podList).NotTo(BeNil())
				isAllTerminated := true
				for _, pod := range podList.Items {
					isAllTerminated = isAllTerminated && pod.Status.Phase == corev1.PodSucceeded
				}
				return isAllTerminated
			}, timeout, interval).Should(BeTrue())
		})

		It("should all of the pods delete finally", func() {
			Eventually(func() bool {
				podList, err := k8sClient.CoreV1().Pods(DefaultNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: selector.String(),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(podList).NotTo(BeNil())
				return len(podList.Items) == 0
			}, timeout, interval).Should(BeTrue())
		})
	})
})
