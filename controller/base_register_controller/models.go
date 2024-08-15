package base_register_controller

import (
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
)

type BuildBaseRegisterControllerConfig struct {
	ClientID string `json:"clientID"`
	Env      string `json:"env"`

	// K8SConfig is the config of k8s client
	K8SConfig *model.K8SConfig

	// Tunnels is the tunnel provider supported
	Tunnels []tunnel.Tunnel
}
