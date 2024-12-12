package tunnel

import (
	"github.com/koupleless/virtual-kubelet/model"
	v1 "k8s.io/api/core/v1"
)

// OnBaseDiscovered is the node discover callback, will start/stop a vnode depends on node state
type OnBaseDiscovered func(model.NodeInfo)

// OnBaseStatusArrived is the node health data callback, will update vnode status to k8s
type OnBaseStatusArrived func(string, model.NodeStatusData)

// OnAllBizStatusArrived is the container status data callback, will update vpod status to k8s
type OnAllBizStatusArrived func(string, []model.BizStatusData)

// OnSingleBizStatusArrived is one container status data callback, will update container-vpod status to k8s
type OnSingleBizStatusArrived func(string, model.BizStatusData)

type Tunnel interface {
	// Key is the identity of Tunnel, will set to node label for special usage
	Key() string

	// Start is the func of tunnel start, please call the callback functions after start
	Start(clientID string, env string) error

	// Ready is the func for check tunnel ready, should return true after tunnel start success
	Ready() bool

	// RegisterCallback is the init func of Tunnel, please complete callback register in this func
	RegisterCallback(OnBaseDiscovered, OnBaseStatusArrived, OnAllBizStatusArrived, OnSingleBizStatusArrived)

	// RegisterNode is the func call when a vnode start successfully, you can implement it on demand
	RegisterNode(initData model.NodeInfo) error

	// UnRegisterNode is the func call when a vnode shutdown successfully, you can implement it on demand
	UnRegisterNode(nodeName string)

	// OnNodeNotReady is the func call when a vnode status turns to not ready, you can implement it on demand
	OnNodeNotReady(nodeName string)

	// FetchHealthData is the func call for vnode to fetch health data , you need to fetch health data and call OnBaseStatusArrived when data arrived
	FetchHealthData(nodeName string) error

	// QueryAllBizStatusData is the func call for vnode to fetch all containers status data , you need to fetch all containers status data and call OnAllBizStatusArrived when data arrived
	QueryAllBizStatusData(nodeName string) error

	// StartBiz is the func calls for vnode to start a biz instance, you need to start container and call OnStartBizResponseArrived when start complete with a response
	StartBiz(nodeName, podKey string, container *v1.Container) error

	// StopBiz is the func calls for vnode to shut down a container , you need to start to shut down container and call OnShutdownContainerResponseArrived when shut down process complete with a response
	StopBiz(nodeName, podKey string, container *v1.Container) error

	// GetBizUniqueKey is the func returns a unique key of a container in a pod, vnode will use this unique key to find target Container status
	GetBizUniqueKey(container *v1.Container) string
}
