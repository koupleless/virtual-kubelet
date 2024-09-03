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

var _ = Describe("VNode Lifecycle Test", func() {

	ctx := context.Background()

	nodeID := "suite-node-node-lifecycle"
	nodeVersion := "1.0.0"
	vnode := &v1.Node{}

	nodeInfo := prepareNode(nodeID, nodeVersion)

	name := utils.FormatNodeName(nodeID)

	Context("node online and deactive finally", func() {
		It("node should become a ready vnode eventually", func() {
			nodeInfo.NodeInfo.Metadata.Status = model.NodeStatusActivated
			tl.PutNode(nodeID, nodeInfo)

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

		It("node should not start again with the same name", func() {
			tl.PutNode(nodeID, nodeInfo)
		})

		It("node should contains custom information after status sync", func() {
			Expect(vnode.Labels[model.LabelKeyOfVNodeName]).To(Equal(nodeID))
			Expect(vnode.Labels[model.LabelKeyOfVNodeVersion]).To(Equal(nodeVersion))
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

		It("node offline with deactive message and finally exit", func() {
			nodeInfo.NodeInfo.Metadata.Status = model.NodeStatusDeactivated
			tl.PutNode(nodeID, nodeInfo)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: name,
				}, vnode)
				return errors.IsNotFound(err)
			}, time.Second*30, time.Second).Should(BeTrue())
		})
	})
	//
	//Context("node online and timeout finally", func() {
	//	It("node should become a ready vnode eventually", func() {
	//		nodeInfo.Metadata.Status = model.NodeStatusActivated
	//		tl.PutNode(nodeID, nodeInfo)
	//
	//		Eventually(func() bool {
	//			err := k8sClient.Get(ctx, types.NamespacedName{
	//				Name: name,
	//			}, vnode)
	//			vnodeReady := false
	//			for _, cond := range vnode.Status.Conditions {
	//				if cond.Type == v1.NodeReady {
	//					vnodeReady = cond.Status == v1.ConditionTrue
	//					break
	//				}
	//			}
	//			return err == nil && vnodeReady
	//		}, time.Second*20, time.Second).Should(BeTrue())
	//		Expect(vnode).NotTo(BeNil())
	//	})
	//
	//	It("node timeout offline need to be not ready", func() {
	//		tl.DeleteNode(nodeID)
	//		Eventually(func() bool {
	//			err := k8sClient.Get(ctx, types.NamespacedName{
	//				Name: name,
	//			}, vnode)
	//			notReady := false
	//			for _, cond := range vnode.Status.Conditions {
	//				if cond.Type == v1.NodeReady && cond.Status == v1.ConditionFalse {
	//					notReady = true
	//				}
	//			}
	//			return err == nil && notReady
	//		}, time.Minute, time.Second).Should(BeTrue())
	//	})
	//
	//	It("node offline with deactive message and finally exit", func() {
	//		nodeInfo.NodeInfo.Metadata.Status = model.NodeStatusDeactivated
	//		tl.PutNode(nodeID, nodeInfo)
	//		Eventually(func() bool {
	//			err := k8sClient.Get(ctx, types.NamespacedName{
	//				Name: name,
	//			}, vnode)
	//			return errors.IsNotFound(err)
	//		}, time.Second*30, time.Second).Should(BeTrue())
	//	})
	//})

})
