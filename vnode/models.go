package vnode

import (
	"github.com/koupleless/virtual-kubelet/tunnel"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BuildVNodeConfig struct {
	// Client is the runtime client instance
	Client client.Client

	// KubeCache is the cache of kube resources
	KubeCache cache.Cache

	// Tunnel is the tunnel of pod management
	Tunnel tunnel.Tunnel

	// NodeID is the base id of base
	NodeID string

	// NodeIP is the ip of base
	NodeIP string

	// NodeHostname is the hostname of base
	NodeHostname string

	// NodeName is the base master biz name
	NodeName string

	// NodeVersion is the base master biz version
	NodeVersion string

	// Env is the runtime env of biz
	Env string

	// CustomTaints is the taint set by tunnel
	CustomTaints []v1.Taint
}
