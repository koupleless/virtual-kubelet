package node_provider

type BuildBaseNodeProviderConfig struct {
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
}
