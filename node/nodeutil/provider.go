package nodeutil

import (
	"github.com/koupleless/virtual-kubelet/node"
	v1 "k8s.io/api/core/v1"
)

// Provider contains the methods required to implement a virtual-kubelet provider.
//
// Errors produced by these methods should implement an interface from
// github.com/virtual-kubelet/virtual-kubelet/errdefs package in order for the
// core logic to be able to understand the type of failure
type Provider interface {
	node.PodLifecycleHandler
}

// ProviderConfig holds objects created by NewNodeFromClient that a provider may need to bootstrap itself.
type ProviderConfig struct {
	// Hack to allow the provider to set things on the node
	// Since the provider is bootstrapped after the node object is configured
	// Primarily this is due to carry-over from the pre-1.0 interfaces that expect the provider instead of the direct *caller* to configure the node.
	Node *v1.Node
}

// NewProviderFunc is used from NewNodeFromClient to bootstrap a provider using the client/listers/etc created there.
// If a nil node provider is returned a default one will be used.
type NewProviderFunc func(ProviderConfig) (Provider, node.NodeProvider, error)
