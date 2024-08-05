package base_node

import (
	"github.com/koupleless/virtual-kubelet/tunnel"
	informer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	lister "k8s.io/client-go/listers/core/v1"
)

type BuildBaseNodeConfig struct {
	// KubeClient is the kube client instance
	KubeClient kubernetes.Interface

	// PodLister is the pod lister
	PodLister lister.PodLister

	// PodInformer is the pod informer
	PodInformer informer.PodInformer

	// Tunnel is the tunnel of pod management
	Tunnel tunnel.Tunnel

	// BaseID is the base id of base
	BaseID string

	// NodeIP is the base ip of base
	NodeIP string

	// TechStack is the base tech stack, default java
	TechStack string

	// BizName is the base master biz name
	BizName string

	// BizVersion is the base master biz version
	BizVersion string

	// Env is the runtime env of biz
	Env string
}
