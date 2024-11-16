package tunnel

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/controller/vnode_controller"
	"github.com/koupleless/virtual-kubelet/model"
	corev1 "k8s.io/api/core/v1"
	"sync"
	"time"
)

var _ Tunnel = &MockTunnel{}

type Node struct {
	model.NodeInfo
	model.NodeStatusData
}

type MockTunnel struct {
	sync.Mutex
	bizStatusStorage map[string]map[string]model.BizStatusData
	nodeStorage      map[string]Node
	NodeNotReady     map[string]bool
	vNodeController  *vnode_controller.VNodeController
}

func NewMockTunnel(vNodeController *vnode_controller.VNodeController) *MockTunnel {
	return &MockTunnel{
		vNodeController: vNodeController,
	}
}

func (m *MockTunnel) OnNodeNotReady(ctx context.Context, nodeID string) {
	m.NodeNotReady[nodeID] = true
	return
}

func (m *MockTunnel) PutNode(ctx context.Context, nodeID string, node Node) {
	m.Lock()
	defer m.Unlock()

	m.nodeStorage[nodeID] = node
	m.OnBaseDiscovered(nodeID, node.NodeInfo, m)
}

func (m *MockTunnel) DeleteNode(nodeID string) {
	m.Lock()
	defer m.Unlock()

	delete(m.nodeStorage, nodeID)
}

func (m *MockTunnel) UpdateBizStatus(ctx context.Context, nodeID, containerKey string, data model.BizStatusData) {
	m.Lock()
	defer m.Unlock()

	bizStatusMap, has := m.bizStatusStorage[nodeID]
	if !has {
		bizStatusMap = map[string]model.BizStatusData{}
	}
	bizStatusMap[containerKey] = data
	m.bizStatusStorage[nodeID] = bizStatusMap
	m.OnSingleBizStatusArrived(nodeID, data)
}

func (m *MockTunnel) Key() string {
	return "mock_tunnel"
}

func (m *MockTunnel) Start(ctx context.Context, clientID string, env string) error {
	m.bizStatusStorage = map[string]map[string]model.BizStatusData{}
	m.nodeStorage = map[string]Node{}
	m.NodeNotReady = map[string]bool{}
	m.startAllBizStatusHeartBeatTask(ctx)
	m.startBaseStatusHeartBeatTask(ctx)
	return nil
}

func (m *MockTunnel) Ready() bool {
	return true
}

func (m *MockTunnel) OnBaseStatusArrived(nodeId string, nodeStatus model.NodeStatusData) {
	m.vNodeController.OnBaseStatusArrived(nodeId, nodeStatus)
}

func (m *MockTunnel) OnBaseDiscovered(nodeId string, nodeInfo model.NodeInfo, tunnel Tunnel) {
	m.vNodeController.OnBaseDiscovered(nodeId, nodeInfo)
}

func (m *MockTunnel) OnAllBizStatusArrived(nodeID string, bizStatuses []model.BizStatusData) {
	m.vNodeController.OnAllBizStatusArrived(nodeID, bizStatuses)
}

func (m *MockTunnel) OnSingleBizStatusArrived(nodeID string, bizStatus model.BizStatusData) {
	m.vNodeController.OnSingleBizStatusArrived(nodeID, bizStatus)
}

func (m *MockTunnel) OnNodeStart(ctx context.Context, nodeID string, initData model.NodeInfo) {
	return
}

func (m *MockTunnel) OnNodeStop(ctx context.Context, nodeID string) {
	return
}

func (m *MockTunnel) FetchHealthData(ctx context.Context, nodeID string) error {
	data, has := m.nodeStorage[nodeID]
	if has {
		m.OnBaseStatusArrived(nodeID, data.NodeStatusData)
	}
	return nil
}

func (m *MockTunnel) QueryAllBizStatusData(ctx context.Context, nodeID string) error {
	_, has := m.nodeStorage[nodeID]
	if has {
		m.OnAllBizStatusArrived(nodeID, convertContainerMap2ContainerList(m.bizStatusStorage[nodeID]))
	}
	return nil
}

func (m *MockTunnel) StartBiz(ctx context.Context, nodeID, podKey string, container *corev1.Container) error {
	m.Lock()
	defer m.Unlock()
	key := utils.GetBizUniqueKey(container)
	containerMap, has := m.bizStatusStorage[nodeID]
	if !has {
		containerMap = map[string]model.BizStatusData{}
	}
	data := model.BizStatusData{
		Key:        key,
		Name:       container.Name,
		PodKey:     podKey,
		State:      string(model.BizStateUnResolved),
		ChangeTime: time.Now(),
		Reason:     "mock_resolved",
		Message:    "mock resolved",
	}
	containerMap[key] = data

	m.bizStatusStorage[nodeID] = containerMap

	// start to biz installation
	m.OnSingleBizStatusArrived(nodeID, data)
	return nil
}

func (m *MockTunnel) StopBiz(ctx context.Context, nodeID, podKey string, container *corev1.Container) error {
	m.Lock()
	defer m.Unlock()
	containerMap := m.bizStatusStorage[nodeID]
	key := utils.GetBizUniqueKey(container)
	data := containerMap[key]
	delete(containerMap, key)
	m.bizStatusStorage[nodeID] = containerMap
	data.State = string(model.BizStateStopped)
	data.ChangeTime = time.Now()
	m.OnSingleBizStatusArrived(nodeID, data)
	return nil
}

func (m *MockTunnel) GetBizUniqueKey(container *corev1.Container) string {
	return utils.GetBizUniqueKey(container)
}

func (m *MockTunnel) startBaseStatusHeartBeatTask(ctx context.Context) {
	go utils.TimedTaskWithInterval(ctx, time.Second*10, func(ctx context.Context) {
		runningVNodes := m.vNodeController.GetRunningVnode()
		for _, runningVNode := range runningVNodes {
			nodeID := runningVNode.GetNodeId()
			vnCtx := context.WithValue(context.Background(), "nodeID", nodeID)
			// Start a new goroutine to fetch node health data every 10 seconds
			go func() {
				log.G(vnCtx).Info("fetch node health data for nodeId ", nodeID)
				err := m.FetchHealthData(vnCtx, nodeID)
				if err != nil {
					log.G(vnCtx).WithError(err).Errorf("Failed to fetch node health info from %s", nodeID)
				}
			}()
		}
	})

}
func (m *MockTunnel) startAllBizStatusHeartBeatTask(ctx context.Context) {
	go utils.TimedTaskWithInterval(ctx, time.Second*15, func(ctx context.Context) {
		runningVNodes := m.vNodeController.GetRunningVnode()
		for _, runningVNode := range runningVNodes {
			nodeID := runningVNode.GetNodeId()
			vnCtx := context.WithValue(ctx, "nodeID", nodeID)
			go func() {
				log.G(vnCtx).Info("query all container status data for nodeId ", nodeID)
				err := m.QueryAllBizStatusData(vnCtx, nodeID)
				if err != nil {
					log.G(vnCtx).WithError(err).Errorf("Failed to query containers info from %s", nodeID)
				}
			}()
		}
	})
}

func convertContainerMap2ContainerList(containerMap map[string]model.BizStatusData) []model.BizStatusData {
	ret := make([]model.BizStatusData, 0)
	for _, container := range containerMap {
		ret = append(ret, container)
	}
	return ret
}
