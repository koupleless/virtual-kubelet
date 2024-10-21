package tunnel

import (
	"context"
	"github.com/koupleless/virtual-kubelet/model"
	v1 "k8s.io/api/core/v1"
)

// OnNodeDiscovered is the node discover callback, will start/stop a vnode depends on node state
type OnNodeDiscovered func(string, model.NodeInfo, Tunnel)

// OnNodeStatusDataArrived is the node health data callback, will update vnode status to k8s
type OnNodeStatusDataArrived func(string, model.NodeStatusData)

// OnQueryAllContainerStatusDataArrived is the container status data callback, will update vpod status to k8s
type OnQueryAllContainerStatusDataArrived func(string, []model.ContainerStatusData)

// OnSingleContainerStatusChanged is one container status data callback, will update container-vpod status to k8s
type OnSingleContainerStatusChanged func(string, model.ContainerStatusData)

type Tunnel interface {
	// Key is the identity of Tunnel, will set to node label for special usage
	Key() string

	// Start is the func of tunnel start, please call the callback functions after start
	Start(ctx context.Context, clientID string, env string) error

	// Ready is the func for check tunnel ready, should return true after tunnel start success
	Ready() bool

	// RegisterCallback is the init func of Tunnel, please complete callback register in this func
	RegisterCallback(OnNodeDiscovered, OnNodeStatusDataArrived, OnQueryAllContainerStatusDataArrived, OnSingleContainerStatusChanged)

	// OnNodeStart is the func call when a vnode start successfully, you can implement it on demand
	OnNodeStart(ctx context.Context, nodeID string, initData model.NodeInfo)

	// OnNodeStop is the func call when a vnode shutdown successfully, you can implement it on demand
	OnNodeStop(ctx context.Context, nodeID string)

	// OnNodeNotReady is the func call when a vnode status turns to not ready, you can implement it on demand
	OnNodeNotReady(ctx context.Context, info model.UnreachableNodeInfo)

	// FetchHealthData is the func call for vnode to fetch health data , you need to fetch health data and call OnNodeStatusDataArrived when data arrived
	FetchHealthData(ctx context.Context, nodeID string) error

	// QueryAllContainerStatusData is the func call for vnode to fetch all containers status data , you need to fetch all containers status data and call OnQueryAllContainerStatusDataArrived when data arrived
	QueryAllContainerStatusData(ctx context.Context, nodeID string) error

	// StartContainer is the func calls for vnode to start a container , you need to start container and call OnStartContainerResponseArrived when start complete with a response
	StartContainer(ctx context.Context, nodeID, podKey string, container *v1.Container) error

	// ShutdownContainer is the func calls for vnode to shut down a container , you need to start to shut down container and call OnShutdownContainerResponseArrived when shut down process complete with a response
	ShutdownContainer(ctx context.Context, nodeID, podKey string, container *v1.Container) error

	// GetContainerUniqueKey is the func returns a unique key of a container in a pod, vnode will use this unique key to find target Container status
	GetContainerUniqueKey(podKey string, container *v1.Container) string
}
