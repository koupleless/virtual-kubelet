package node_provider

type BuildBaseNodeProviderConfig struct {
	// NodeIP is the ip of the node
	NodeIP string `json:"nodeIP"`

	// TechStack is the underlying tech stack of runtime
	TechStack string `json:"techStack"`

	// BizName is the master biz name of runtime
	BizName string `json:"bizName"`

	// Version is the version of ths underlying runtime
	Version string `json:"version"`

	// ENV is the env of base runtime
	Env string `json:"env"`
}
