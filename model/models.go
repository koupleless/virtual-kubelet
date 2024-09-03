package model

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

// NetworkInfo is the network of vnode, will be set into node addresses
type NetworkInfo struct {
	NodeIP   string `json:"nodeIP"`
	HostName string `json:"hostName"`
}

// NodeMetadata is the base data of a vnode, will be transfer to default labels of a vnode
type NodeMetadata struct {
	// Name is the name of vnode
	Name string `json:"name"`
	// Version is the version of vnode
	Version string `json:"version"`
	// Status is the curr status of vnode
	Status NodeStatus `json:"status"`
}

// QueryBaselineRequest is the request parameters of query baseline func
type QueryBaselineRequest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	CustomLabels map[string]string `json:"customLabels"`
	CustomTaints []v1.Taint        `json:"customTaints"`
}

// NodeInfo is the data of node info.
type NodeInfo struct {
	Metadata     NodeMetadata `json:"metadata"`
	NetworkInfo  NetworkInfo  `json:"networkInfo"`
	CustomTaints []v1.Taint   `json:"customTaints"`
}

// NodeResource is the data of node resource
type NodeResource struct {
	Capacity    resource.Quantity `json:"capacity"`
	Allocatable resource.Quantity `json:"allocatable"`
}

// NodeStatusData is the status of a node, you can set some custom attributes in this data structure
type NodeStatusData struct {
	Resources         map[v1.ResourceName]NodeResource `json:"resources"`
	CustomLabels      map[string]string                `json:"customLabels"`
	CustomAnnotations map[string]string                `json:"customAnnotations"`
	CustomConditions  []v1.NodeCondition               `json:"customConditions"`
}

// ContainerStatusData is the status data of a container
type ContainerStatusData struct {
	// Key generated by tunnel, need to be the same as Tunnel GetContainerUniqueKey of same container
	Key string `json:"key"`
	// Name container name
	Name string `json:"name"`
	// PodKey is the key of pod which contains this container ,you can set it to PodKeyAll to present a shared container
	PodKey     string         `json:"podKey"`
	State      ContainerState `json:"state"`
	ChangeTime time.Time      `json:"changeTime"`
	Reason     string         `json:"reason"`
	Message    string         `json:"message"`
}

// PodStatusData is the status data of a container
type PodStatusData struct {
	CustomLabels      map[string]string `json:"customLabels"`
	CustomAnnotations map[string]string `json:"customAnnotations"`
}

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

type BuildVNodeConfig struct {
	// Client is the runtime client instance
	Client client.Client

	// KubeCache is the cache of kube resources
	KubeCache cache.Cache

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

type BuildVNodeControllerConfig struct {
	ClientID string

	Env string

	// VPodIdentity is the vpod special value of model.LabelKeyOfComponent
	VPodIdentity string
}
