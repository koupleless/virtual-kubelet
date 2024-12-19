package suite

import (
	"context"
	"github.com/koupleless/virtual-kubelet/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v12 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"time"
)

var _ = Describe("VNode Lifecycle Test", func() {

	ctx := context.Background()

	nodeName := "suite-node-node-lifecycle"
	nodeVersion := "1.0.0"
	baseName := "suite-base-name"
	clusterName := "suite-cluster-name"
	node := &v1.Node{}

	nodeInfo := prepareNode(nodeName, nodeVersion, baseName, clusterName)

	Context("node online and deactive finally", func() {
		It("node should become a ready node eventually", func() {
			nodeInfo.NodeInfo.State = model.NodeStateActivated
			tl.PutNode(ctx, nodeName, nodeInfo)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
				}, node)
				vnodeReady := false
				for _, cond := range node.Status.Conditions {
					if cond.Type == v1.NodeReady {
						vnodeReady = cond.Status == v1.ConditionTrue
						break
					}
				}
				return err == nil && vnodeReady
			}, time.Second*50, time.Second).Should(BeTrue())
		})

		It("node online and should get the lease", func() {
			Eventually(func() bool {
				lease := &v12.Lease{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      nodeName,
					Namespace: v1.NamespaceNodeLease,
				}, lease)
				return err == nil
			}, time.Second*30, time.Second).Should(BeTrue())
		})

		It("node should not start again with the same name", func() {
			tl.PutNode(ctx, nodeName, nodeInfo)
		})

		It("node should contains custom information after status sync", func() {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name: nodeName,
			}, node)

			Expect(err).To(BeNil())
			Expect(node.Labels[model.LabelKeyOfBaseName]).To(Equal(baseName))
			Expect(node.Labels[model.LabelKeyOfBaseVersion]).To(Equal(nodeVersion))
			Expect(node.Labels[model.LabelKeyOfBaseClusterName]).To(Equal(clusterName))
			Expect(node.Labels[testKey]).To(Equal(testValue))
			Expect(node.Labels[model.LabelKeyOfEnv]).To(Equal(env))
			Expect(node.Annotations[testKey]).To(Equal(testValue))
			existTestCond := false
			for _, cond := range node.Status.Conditions {
				if string(cond.Type) == testKey {
					existTestCond = true
					break
				}
			}
			Expect(existTestCond).To(BeTrue())
			existTestTaint := false
			for _, taint := range node.Spec.Taints {
				if taint.Key == testKey && taint.Value == testValue {
					existTestTaint = true
					break
				}
			}
			Expect(existTestTaint).To(BeTrue())
		})

		It("node offline with deactive message and finally exit", func() {
			nodeInfo.NodeInfo.State = model.NodeStateDeactivated
			tl.PutNode(ctx, nodeName, nodeInfo)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
				}, node)
				lease := &v12.Lease{}
				leaseErr := k8sClient.Get(ctx, types.NamespacedName{
					Name:      nodeName,
					Namespace: v1.NamespaceNodeLease,
				}, lease)
				return errors.IsNotFound(err) && errors.IsNotFound(leaseErr)
			}, time.Second*30, time.Second).Should(BeTrue())
		})
	})

	Context("node online and timeout finally", func() {
		It("node should become a ready node eventually", func() {
			nodeInfo.State = model.NodeStateActivated
			tl.PutNode(ctx, nodeName, nodeInfo)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
				}, node)
				vnodeReady := false
				for _, cond := range node.Status.Conditions {
					if cond.Type == v1.NodeReady {
						vnodeReady = cond.Status == v1.ConditionTrue
						break
					}
				}
				return err == nil && vnodeReady
			}, time.Second*20, time.Second).Should(BeTrue())
			Expect(node).NotTo(BeNil())
		})

		// TODO: enable this by update node status when deactivated
		It("node offline with deactive message and finally exit", func() {
			nodeInfo.NodeInfo.State = model.NodeStateDeactivated
			tl.PutNode(ctx, nodeName, nodeInfo)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
				}, node)
				return errors.IsNotFound(err)
			}, time.Minute*2, time.Second).Should(BeTrue())
		})
	})
})
