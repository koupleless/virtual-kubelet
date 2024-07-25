package base_register_controller

import (
	"github.com/koupleless/virtual-kubelet/tunnel"
	"time"
)

type BuildBaseRegisterControllerConfig struct {
	ClientID string `json:"clientID"`

	// K8SConfig is the config of k8s client
	K8SConfig *K8SConfig

	// Tunnels is the tunnel provider supported
	Tunnels []tunnel.Tunnel
}

type K8SConfig struct {
	// KubeConfigPath is the path of k8s client
	KubeConfigPath string

	// InformerSyncPeriod
	InformerSyncPeriod time.Duration
}
