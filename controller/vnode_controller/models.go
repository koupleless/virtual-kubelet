package vnode_controller

import (
	"github.com/koupleless/virtual-kubelet/tunnel"
)

type BuildVNodeControllerConfig struct {
	ClientID string

	Env string

	// VPodIdentity is the vpod special value of model.LabelKeyOfComponent
	VPodIdentity string

	// Tunnels is the tunnel provider supported
	Tunnels []tunnel.Tunnel
}
