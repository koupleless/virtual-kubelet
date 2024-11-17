package suite

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/vnode_controller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var testEnv *envtest.Environment
var k8sClient client.Client
var tl tunnel.MockTunnel

const (
	clientID     = "suite-suite"
	env          = "suite-suite"
	vPodIdentity = "vpod"
	testKey      = "virtual-kubelet.kouleless.io/custom"
	testValue    = "suite-suite"
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping suite environment")
	testEnv = &envtest.Environment{}

	var err error

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = scheme.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	ctx := context.Background()

	vnodeController, err := vnode_controller.NewVNodeController(&model.BuildVNodeControllerConfig{
		ClientID:     clientID,
		Env:          env,
		VPodIdentity: vPodIdentity,
		IsCluster:    true,
	}, &tl)

	err = vnodeController.SetupWithManager(ctx, k8sManager)

	Expect(err).ToNot(HaveOccurred())

	tunnels := []tunnel.Tunnel{
		&tl,
	}
	for _, t := range tunnels {
		err = t.Start(ctx, clientID, env)
		if err != nil {
			log.G(ctx).WithError(err).Error("failed to start tunnel", t.Key())
		} else {
			log.G(ctx).Info("Tunnel started: ", t.Key())
		}
	}

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

	go func() {
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

	time.Sleep(5 * time.Second)
})

var _ = AfterSuite(func() {
	By("tearing down the suite environment")
	testEnv.Stop()
})

func prepareNode(name, version string) tunnel.Node {
	return tunnel.Node{
		NodeInfo: model.NodeInfo{
			Metadata: model.NodeMetadata{
				Name:    name,
				Version: version,
			},
			NetworkInfo: model.NetworkInfo{
				HostName: name,
			},
			CustomTaints: []v1.Taint{
				{
					Key:    testKey,
					Value:  testValue,
					Effect: v1.TaintEffectNoExecute,
				},
			},
			State: model.NodeStateActivated,
		},
		NodeStatusData: model.NodeStatusData{
			Resources: map[v1.ResourceName]model.NodeResource{
				v1.ResourceMemory: {
					Capacity:    *resource.NewQuantity(10240, resource.BinarySI),
					Allocatable: *resource.NewQuantity(10240, resource.BinarySI),
				},
			},
			CustomLabels: map[string]string{
				testKey: testValue,
			},
			CustomAnnotations: map[string]string{
				testKey: testValue,
			},
			CustomConditions: []v1.NodeCondition{
				{
					Type:               testKey,
					Status:             v1.ConditionTrue,
					LastHeartbeatTime:  metav1.Now(),
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}
}

func prepareBasicPod(name, namespace, nodeName string) v1.Pod {
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				model.LabelKeyOfComponent: vPodIdentity,
			},
		},
		Spec: v1.PodSpec{
			NodeName: nodeName,
			Containers: []v1.Container{
				{
					Name:  "suite-biz1",
					Image: "suite-biz1.jar",
					Env: []v1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "1.0.0",
						},
					},
				},
				{
					Name:  "suite-biz2",
					Image: "suite-biz2.jar",
					Env: []v1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "1.0.0",
						},
					},
				},
			},
			Tolerations: []v1.Toleration{
				{
					Key:      testKey,
					Operator: v1.TolerationOpEqual,
					Value:    testValue,
					Effect:   v1.TaintEffectNoExecute,
				},
				{
					Key:      model.TaintKeyOfVnode,
					Operator: v1.TolerationOpEqual,
					Value:    "True",
					Effect:   v1.TaintEffectNoExecute,
				},
				{
					Key:      model.TaintKeyOfEnv,
					Operator: v1.TolerationOpEqual,
					Value:    env,
					Effect:   v1.TaintEffectNoExecute,
				},
			},
		},
	}
}
