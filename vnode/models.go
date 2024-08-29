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

	// Tunnel is the tunnel of container and node management
	Tunnel tunnel.Tunnel

	// NodeID is the unique id of node, should be unique key of system
	NodeID string

	// NodeIP is the ip of node
	NodeIP string

	// NodeHostname is the hostname of node
	NodeHostname string

	// NodeName is the name of node, will set to node's label
	NodeName string

	// NodeVersion is the version of node, will set to node's label
	NodeVersion string

	// Env is the runtime env of virtual-kubelet, will set to node created by virtual kubelet
	Env string

	// CustomTaints is the taint set by tunnel
	CustomTaints []v1.Taint
}
