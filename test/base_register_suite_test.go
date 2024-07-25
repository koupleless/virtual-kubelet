package test

import (
	"github.com/koupleless/virtual-kubelet/common/testutil/base"
	"github.com/koupleless/virtual-kubelet/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

var _ = Describe("Base Register", func() {

	const timeout = time.Second * 60

	const interval = time.Second * 3

	nodeId := "test-base"

	var mockBase *base.MockMqttBase

	Context("base online test", func() {
		It("mock base should start successfully", func() {
			mockBase = base.NewBaseMock(nodeId, "base", "1.1.1", baseMqttClient)
			go mockBase.Run()
		})

		It("should has target node", func() {
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Nodes().Get(mainContext, model.VIRTUAL_NODE_NAME_PREFIX+nodeId, metav1.GetOptions{})
				return !errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("base offline test", func() {
		It("mock base end successfully", func() {
			if mockBase != nil {
				mockBase.Exit()
			}
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Nodes().Get(mainContext, model.VIRTUAL_NODE_NAME_PREFIX+nodeId, metav1.GetOptions{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

})
