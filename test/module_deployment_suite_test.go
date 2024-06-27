package test

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"path"
	"time"
)

// using deployment to achieve module non-peer deployment
var _ = Describe("Module Deployment", func() {

	const timeout = time.Second * 90

	const interval = time.Second * 3

	ctx := context.WithValue(context.Background(), "env", "module-deployment")
	moduleDeploymentNonPeerYamlFilePath := path.Join("samples", "module_deployment.yaml")

	var selector labels.Selector
	var err error
	moduleDeploymentNonPeer, err := getDeploymentFromYamlFile(moduleDeploymentNonPeerYamlFilePath)

	Context("create deployment", func() {
		It("should create successfully", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(moduleDeploymentNonPeer).NotTo(BeNil())

			requirement, err := labels.NewRequirement("app", selection.Equals, []string{moduleDeploymentNonPeer.Name})
			Expect(err).ToNot(HaveOccurred())
			selector = labels.NewSelector().Add(*requirement)

			_, err = k8sClient.AppsV1().Deployments(DefaultNamespace).Create(ctx, moduleDeploymentNonPeer, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should all of the pods change to running status finally", func() {
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
				return isAllReady && len(podList.Items) == int(*moduleDeploymentNonPeer.Spec.Replicas)
			}, timeout, interval).Should(BeTrue())
		})
	})

	var currScale *autoscalingv1.Scale
	Context("scale up deployment", func() {
		It("should scale up successfully", func() {
			currScale, err = k8sClient.AppsV1().Deployments(DefaultNamespace).GetScale(ctx, moduleDeploymentNonPeer.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(currScale).NotTo(BeNil())

			// scale up 2 replicas
			currScale.Spec.Replicas += 2
			scaleResult, err := k8sClient.AppsV1().Deployments(DefaultNamespace).UpdateScale(ctx, moduleDeploymentNonPeer.Name, currScale, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(scaleResult).NotTo(BeNil())
			currScale = scaleResult
		})

		It("should create enough pods", func() {
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
				return isAllReady && len(podList.Items) == int(currScale.Spec.Replicas)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("update deployment module", func() {
		It("should update successfully", func() {
			// get current deployment info
			moduleDeploymentNonPeer, err := k8sClient.AppsV1().Deployments(DefaultNamespace).Get(context.Background(), moduleDeploymentNonPeer.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(moduleDeploymentNonPeer).NotTo(BeNil())
			// publish new template
			moduleDeploymentNonPeer.Spec.Template.Spec.Containers[0].Name = "biz2"
			moduleDeploymentNonPeer.Spec.Template.Spec.Containers[0].Image = "https://serverless-opensource.oss-cn-shanghai.aliyuncs.com/module-packages/stable/biz2-web-single-host-0.0.1-SNAPSHOT-ark-biz.jar"
			updatedDeployment, err := k8sClient.AppsV1().Deployments(DefaultNamespace).Update(ctx, moduleDeploymentNonPeer, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedDeployment).NotTo(BeNil())
			Expect(len(updatedDeployment.Spec.Template.Spec.Containers)).To(Equal(1))
			Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Name).To(Equal("biz2"))
		})

		It("should have correct revisions", func() {
			replicaSetRequirement, err := labels.NewRequirement("app", selection.Equals, []string{moduleDeploymentNonPeer.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(replicaSetRequirement).NotTo(BeNil())
			replicaSelector := labels.NewSelector().Add(*replicaSetRequirement)
			replicaSetList, err := k8sClient.AppsV1().ReplicaSets(DefaultNamespace).List(ctx, metav1.ListOptions{LabelSelector: replicaSelector.String()})
			Expect(err).NotTo(HaveOccurred())
			Expect(replicaSetList).NotTo(BeNil())
			// a new and the old one
			Expect(len(replicaSetList.Items)).To(Equal(2))
		})

		It("in rolling update, should contains new pod and old pod at the same time", func() {
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
			Expect(len(numOfPodContainerName)).To(Equal(2))
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
				return isAllReady && len(podList.Items) == int(currScale.Spec.Replicas) && len(numOfPodContainerName) == 1 && numOfPodContainerName["biz2"] != 0
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("scale down deployment", func() {
		It("should scale down successfully", func() {
			currScale, err = k8sClient.AppsV1().Deployments(DefaultNamespace).GetScale(ctx, moduleDeploymentNonPeer.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(currScale).NotTo(BeNil())

			// scale up 2 replicas
			currScale.Spec.Replicas -= 2
			scaleResult, err := k8sClient.AppsV1().Deployments(DefaultNamespace).UpdateScale(ctx, moduleDeploymentNonPeer.Name, currScale, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(scaleResult).NotTo(BeNil())
		})

		It("should delete enough pods", func() {
			time.Sleep(time.Second * 5)
			Eventually(func() bool {
				podList, err := k8sClient.CoreV1().Pods(DefaultNamespace).List(ctx, metav1.ListOptions{
					LabelSelector: selector.String(),
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(podList).NotTo(BeNil())
				return len(podList.Items) == int(currScale.Spec.Replicas)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("delete deployment", func() {
		It("should all of the pods be terminating status", func() {
			Expect(k8sClient.AppsV1().Deployments(DefaultNamespace).Delete(ctx, moduleDeploymentNonPeer.Name, metav1.DeleteOptions{})).NotTo(HaveOccurred())

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
