package node_provider

import v1 "k8s.io/api/core/v1"

type BuildVNodeProviderConfig struct {
	// NodeIP is the ip of the node
	NodeIP string `json:"nodeIP"`

	// NodeHostname is the hostname of the node
	NodeHostname string `json:"nodeHostname"`

	// Name is the master biz name of runtime
	Name string `json:"name"`

	// Version is the version of ths underlying runtime
	Version string `json:"version"`

	// ENV is the env of base runtime
	Env string `json:"env"`

	// CustomTaints is the taints set by tunnel
	CustomTaints []v1.Taint `json:"customTaints"`
}
