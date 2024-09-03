package suite

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("VPod Lifecycle Test", func() {
	ctx := context.Background()

	nodeID := "suite-node-pod-lifecycle"
	nodeVersion := "1.0.0"
	vnode := &v1.Node{}

	podName := "suite-pod"
	podNamespace := "default"

	nodeInfo := prepareNode(nodeID, nodeVersion)
	basicPod := prepareBasicPod(podName, podNamespace, utils.FormatNodeName(nodeID))

	Context("pod publish and status sync", func() {
		It("node should be ready", func() {
			nodeInfo.NodeInfo.Metadata.Status = model.NodeStatusActivated
			tl.PutNode(nodeID, nodeInfo)
			name := utils.FormatNodeName(nodeID)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: name,
				}, vnode)
				vnodeReady := false
				for _, cond := range vnode.Status.Conditions {
					if cond.Type == v1.NodeReady {
						vnodeReady = cond.Status == v1.ConditionTrue
						break
					}
				}
				return err == nil && vnodeReady
			}, time.Second*20, time.Second).Should(BeTrue())
		})

		It("pod publish and should be scheduled", func() {
			err := k8sClient.Create(ctx, &basicPod)
			Expect(err).To(BeNil())
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodPending && podFromKubernetes.Spec.NodeName == utils.FormatNodeName(nodeID)
			}, time.Second*20, time.Second).Should(BeTrue())
		})

		It("when all container's status changes to ready, pod should become ready", func() {
			for _, container := range basicPod.Spec.Containers {
				podKey := utils.GetPodKey(&basicPod)
				key := tl.GetContainerUniqueKey(podKey, &container)
				tl.PutContainer(nodeID, key, model.ContainerStatusData{
					Key:        key,
					Name:       container.Name,
					PodKey:     podKey,
					State:      model.ContainerStateActivated,
					ChangeTime: time.Now(),
				})
			}
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodRunning
			}, time.Second*30, time.Second).Should(BeTrue())
		})

		It("when one container's status changes to deactived, pod should become unready", func() {
			container := basicPod.Spec.Containers[0]
			podKey := utils.GetPodKey(&basicPod)
			key := tl.GetContainerUniqueKey(podKey, &container)
			tl.PutContainer(nodeID, key, model.ContainerStatusData{
				Key:        key,
				Name:       container.Name,
				PodKey:     podKey,
				State:      model.ContainerStateDeactivated,
				ChangeTime: time.Now(),
			})
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodRunning && podFromKubernetes.Status.Conditions[0].Status == v1.ConditionFalse
			}, time.Second*20, time.Second).Should(BeTrue())
		})

		It("update pod containers, pod should be pending", func() {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: basicPod.Namespace,
				Name:      basicPod.Name,
			}, &basicPod)
			Expect(err).To(BeNil())
			basicPod.Spec.Containers[0].Image = "test-container-modify-image"
			err = k8sClient.Update(ctx, &basicPod)
			Expect(err).To(BeNil())
			Expect(err).To(BeNil())
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodPending
			}, time.Second*20, time.Second).Should(BeTrue())
		})

		It("delete pod, all containers should shutdown, pod should finally deleted from k8s", func() {
			err := k8sClient.Delete(ctx, &basicPod)
			Expect(err).To(BeNil())
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				return errors.IsNotFound(err)
			}, time.Second*20, time.Second).Should(BeTrue())
		})

		It("node offline", func() {
			nodeInfo.Metadata.Status = model.NodeStatusDeactivated
			tl.PutNode(nodeID, nodeInfo)
			Eventually(func() bool {
				node := &v1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: utils.FormatNodeName(nodeID),
				}, node)
				return errors.IsNotFound(err)
			}, time.Second*20, time.Second).Should(BeTrue())
		})
	})

})
