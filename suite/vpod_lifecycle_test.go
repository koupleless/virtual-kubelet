package suite

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("VPod Lifecycle Test", func() {
	ctx := context.Background()

	nodeName := "suite-node-pod-lifecycle"
	nodeVersion := "1.0.0"
	clusterName := "suite-cluster-name"
	vnode := &v1.Node{}

	podName := "suite-pod"
	podNamespace := "default"

	nodeInfo := prepareNode(nodeName, nodeVersion, clusterName)
	basicPod := prepareBasicPod(podName, podNamespace, nodeName)

	tasks := []string{
		"node should be ready.",
		"pod publish and should be scheduled.",
		"when all container's status changes to ready, pod should become ready.",
		"when one container's status changes to deactived, pod should become unready.",
		"when this container's status changes to activated, but wrong pod key, pod should not change status.",
		"when this container's status changes to activated, with right pod key, pod should change status.",
		"update pod all containers, pod should be shutdown but not support to start new container, so status should be Succeeded.",
		"delete pod, all containers should shutdown, pod should finally deleted from k8s.",
		"node offline.",
	}

	Context("pod publish and status sync", func() {
		It(tasks[0], func() {
			nodeInfo.NodeInfo.State = model.NodeStateActivated
			tl.PutNode(ctx, nodeName, nodeInfo)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
				}, vnode)
				vnodeReady := false
				logrus.WithContext(ctx).Infof("%s node status: %v", tasks[0], vnode.Status.Conditions)
				for _, cond := range vnode.Status.Conditions {
					if cond.Type == v1.NodeReady {
						vnodeReady = cond.Status == v1.ConditionTrue
						break
					}
				}
				return err == nil && vnodeReady
			}, time.Second*10, time.Second).Should(BeTrue())
		})

		It(tasks[1], func() {
			err := k8sClient.Create(ctx, &basicPod)
			Expect(err).To(BeNil())
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)

				logrus.WithContext(ctx).Infof("%s pod status: %v", tasks[1], podFromKubernetes.Status.Phase)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodPending && podFromKubernetes.Spec.NodeName == nodeName
			}, time.Second*10, time.Second).Should(BeTrue())
		})

		It(tasks[2], func() {
			for _, container := range basicPod.Spec.Containers {
				podKey := utils.GetPodKey(&basicPod)
				key := tl.GetBizUniqueKey(&container)
				time.Sleep(time.Second)
				tl.UpdateBizStatus(ctx, nodeName, key, model.BizStatusData{
					Key:        key,
					Name:       container.Name,
					PodKey:     podKey,
					State:      string(model.BizStateActivated),
					ChangeTime: time.Now(),
				})
			}
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				logrus.WithContext(ctx).Infof("%s pod status: %v", tasks[2], podFromKubernetes.Status.Phase)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodRunning && podFromKubernetes.Status.ContainerStatuses[0].Ready == true
			}, time.Second*20, time.Second).Should(BeTrue())
		})

		It(tasks[3], func() {
			container := basicPod.Spec.Containers[0]
			podKey := utils.GetPodKey(&basicPod)
			key := tl.GetBizUniqueKey(&container)
			tl.UpdateBizStatus(ctx, nodeName, key, model.BizStatusData{
				Key:        key,
				Name:       container.Name,
				PodKey:     podKey,
				State:      string(model.BizStateDeactivated),
				ChangeTime: time.Now(),
			})
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				logrus.WithContext(ctx).Infof("%s pod status: %v", tasks[3], podFromKubernetes.Status.Phase)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodRunning && podFromKubernetes.Status.Conditions[0].Status == v1.ConditionFalse
			}, time.Second*10, time.Second).Should(BeTrue())
		})

		It(tasks[4], func() {
			container := basicPod.Spec.Containers[0]
			podKey := utils.GetPodKey(&basicPod) + "-wrong"
			key := tl.GetBizUniqueKey(&container)
			tl.UpdateBizStatus(ctx, nodeName, key, model.BizStatusData{
				Key:        key,
				Name:       container.Name,
				PodKey:     podKey,
				State:      string(model.BizStateActivated),
				ChangeTime: time.Now(),
			})
			time.Sleep(time.Second)
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				logrus.WithContext(ctx).Infof("%s pod status: %v", tasks[4], podFromKubernetes.Status.Phase)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodRunning && podFromKubernetes.Status.Conditions[0].Status == v1.ConditionFalse
			}, time.Second*10, time.Second).Should(BeTrue())
		})

		It(tasks[5], func() {
			container := basicPod.Spec.Containers[0]
			podKey := utils.GetPodKey(&basicPod)
			key := tl.GetBizUniqueKey(&container)
			tl.UpdateBizStatus(ctx, nodeName, key, model.BizStatusData{
				Key:        key,
				Name:       container.Name,
				PodKey:     podKey,
				State:      string(model.BizStateActivated),
				ChangeTime: time.Now(),
			})
			time.Sleep(time.Second)
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				logrus.WithContext(ctx).Infof("%s pod status: %v", tasks[5], podFromKubernetes.Status.Phase)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodRunning && podFromKubernetes.Status.Conditions[0].Status == v1.ConditionTrue
			}, time.Second*10, time.Second).Should(BeTrue())
		})

		It(tasks[6], func() {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: basicPod.Namespace,
				Name:      basicPod.Name,
			}, &basicPod)
			Expect(err).To(BeNil())
			basicPod.Spec.Containers[0].Image = "suite-biz1-updated.jar"
			basicPod.Spec.Containers[1].Image = "suite-biz2-updated.jar"
			err = k8sClient.Update(ctx, &basicPod)
			Expect(err).To(BeNil())
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				logrus.WithContext(ctx).Infof("%s pod status: %v", tasks[6], podFromKubernetes.Status.Phase)
				return err == nil && podFromKubernetes.Status.Phase == v1.PodSucceeded
			}, time.Second*20, time.Second).Should(BeTrue())
		})

		It(tasks[7], func() {
			err := k8sClient.Delete(ctx, &basicPod)
			Expect(err).To(BeNil())
			Eventually(func() bool {
				podFromKubernetes := &v1.Pod{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: basicPod.Namespace,
					Name:      basicPod.Name,
				}, podFromKubernetes)
				logrus.WithContext(ctx).Infof("%s pod status: %v", tasks[7], podFromKubernetes.Status.Phase)
				return errors.IsNotFound(err)
			}, time.Second*20, time.Second).Should(BeTrue())
		})

		It(tasks[8], func() {
			nodeInfo.State = model.NodeStateDeactivated
			tl.PutNode(ctx, nodeName, nodeInfo)
			Eventually(func() bool {
				node := &v1.Node{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
				}, node)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Second).Should(BeTrue())
		})
	})

})
