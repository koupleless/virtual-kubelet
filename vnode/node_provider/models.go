package node_provider

import v1 "k8s.io/api/core/v1"

type BuildVNodeProviderConfig struct {
	// NodeIP is the ip of the node
	NodeIP string `json:"nodeIP"`

	// NodeHostname is the hostname of the node
	NodeHostname string `json:"nodeHostname"`

	// Name is the node name, will be sent to utils.FormatNodeName to construct vnode name, and Name will be set to label
	Name string `json:"name"`

	// Name is the node version, will be set to label
	Version string `json:"version"`

	// ENV is the env of node, will be set to label
	Env string `json:"env"`

	// CustomTaints is the taints set by tunnel
	CustomTaints []v1.Taint `json:"customTaints"`
}
