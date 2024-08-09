package module_deployment_controller

import (
	"k8s.io/client-go/kubernetes"
	"time"
)

type BuildModuleDeploymentControllerConfig struct {
	ClientID string `json:"clientID"`
	Env      string `json:"env"`

	// K8SConfig is the config of k8s client
	K8SConfig *K8SConfig
}

type K8SConfig struct {
	// KubeClient
	KubeClient kubernetes.Interface

	// InformerSyncPeriod
	InformerSyncPeriod time.Duration
}
