package tunnel

import (
	"context"
	"github.com/koupleless/virtual-kubelet/model"
	v1 "k8s.io/api/core/v1"
)

// OnNodeDiscovered base discover callback, will start a vnode
type OnNodeDiscovered func(string, model.NodeInfo, Tunnel)

// OnNodeStatusDataArrived base health data callback, will update vnode status to k8s
type OnNodeStatusDataArrived func(string, model.NodeStatusData)

// OnQueryAllContainerStatusDataArrived biz data callback, will update vpod status to k8s
type OnQueryAllContainerStatusDataArrived func(string, []model.ContainerStatusData)

// OnStartContainerResponseArrived biz install result callback, will update biz-vpod status to k8s
type OnStartContainerResponseArrived func(string, model.ContainerOperationResponseData)

// OnShutdownContainerResponseArrived biz uninstall result callback, will update biz-vpod status to k8s
type OnShutdownContainerResponseArrived func(string, model.ContainerOperationResponseData)

// QueryContainersBaseline func of query baseline, will return peer deployment baseline
type QueryContainersBaseline func(info model.QueryBaselineRequest) []*v1.Container

type Tunnel interface {
	// Key is the identity of Tunnel, will set to node label for special usage
	Key() string

	// Start is the func of Tunnel Start processing
	Start(ctx context.Context, clientID string, env string) error

	Ready() bool

	// RegisterCallback is the init func of Tunnel, please complete resources init and callback register in this func
	RegisterCallback(OnNodeDiscovered, OnNodeStatusDataArrived, OnQueryAllContainerStatusDataArrived, OnStartContainerResponseArrived, OnShutdownContainerResponseArrived)

	// RegisterQuery is the init func of Tunnel, please complete resources init and callback register in this func
	RegisterQuery(QueryContainersBaseline)

	// OnNodeStart is the func call when a vnode start successfully, you can implement it on demand
	OnNodeStart(ctx context.Context, nodeID string)

	// OnNodeStop is the func call when a vnode shutdown successfully, you can implement it on demand
	OnNodeStop(ctx context.Context, nodeID string)

	// FetchHealthData is the func call for vnode to fetch health data , you need to fetch health data and call OnNodeStatusDataArrived when data arrived
	FetchHealthData(context.Context, string) error

	// QueryAllBizData is the func call for vnode to fetch all biz data , you need to fetch all biz data and call OnQueryAllContainerStatusDataArrived when data arrived
	QueryAllContainerStatusData(ctx context.Context, nodeID string) error

	// InstallBiz is the func call for vnode to install a biz , you need to start to install biz and call OnStartContainerResponseArrived when install complete with a response
	StartContainer(ctx context.Context, nodeID, podKey string, container *v1.Container) error

	// UninstallBiz is the func call for vnode to uninstall a biz , you need to start to uninstall biz and call OnShutdownContainerResponseArrived when uninstall complete with a response
	StopContainer(ctx context.Context, nodeID, podKey string, container *v1.Container) error

	GetContainerUniqueKey(podKey string, container *v1.Container) string
}
