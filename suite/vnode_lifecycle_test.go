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
	clusterName := "suite-cluster-name"
	vnode := &v1.Node{}

	nodeInfo := prepareNode(nodeName, nodeVersion, clusterName)

	Context("node online and deactive finally", func() {
		It("node should become a ready vnode eventually", func() {
			nodeInfo.NodeInfo.State = model.NodeStateActivated
			tl.PutNode(ctx, nodeName, nodeInfo)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
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
			Expect(vnode.Labels[model.LabelKeyOfVNodeName]).To(Equal(nodeName))
			Expect(vnode.Labels[model.LabelKeyOfVNodeVersion]).To(Equal(nodeVersion))
			Expect(vnode.Labels[model.LabelKeyOfVNodeClusterName]).To(Equal(clusterName))
			Expect(vnode.Labels[testKey]).To(Equal(testValue))
			Expect(vnode.Labels[model.LabelKeyOfEnv]).To(Equal(env))
			Expect(vnode.Annotations[testKey]).To(Equal(testValue))
			existTestCond := false
			for _, cond := range vnode.Status.Conditions {
				if string(cond.Type) == testKey {
					existTestCond = true
				}
			}
			Expect(existTestCond).To(BeTrue())
			existTestTaint := false
			for _, taint := range vnode.Spec.Taints {
				if taint.Key == testKey && taint.Value == testValue {
					existTestTaint = true
				}
			}
			Expect(existTestTaint).To(BeTrue())
		})

		It("node not ready should send not ready to mock tunnel", func() {
			tl.DeleteNode(nodeName)
			Eventually(func() bool {
				return tl.NodeNotReady[nodeName]
			}, time.Second*50, time.Second).Should(BeTrue())
		})

		It("node offline with deactive message and finally exit", func() {
			nodeInfo.NodeInfo.State = model.NodeStateDeactivated
			tl.PutNode(ctx, nodeName, nodeInfo)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
				}, vnode)
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
		It("node should become a ready vnode eventually", func() {
			nodeInfo.State = model.NodeStateActivated
			tl.PutNode(ctx, nodeName, nodeInfo)

			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
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
			Expect(vnode).NotTo(BeNil())
		})

		It("node offline with deactive message and finally exit", func() {
			nodeInfo.NodeInfo.State = model.NodeStateDeactivated
			tl.PutNode(ctx, nodeName, nodeInfo)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: nodeName,
				}, vnode)
				return errors.IsNotFound(err)
			}, time.Second*30, time.Second).Should(BeTrue())
		})
	})

})
