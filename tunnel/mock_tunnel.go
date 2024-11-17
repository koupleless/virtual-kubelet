package tunnel

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/utils"
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
	OnBaseDiscovered
	OnBaseStatusArrived
	OnSingleBizStatusArrived
	OnAllBizStatusArrived

	bizStatusStorage map[string]map[string]model.BizStatusData
	nodeStorage      map[string]Node
	NodeNotReady     map[string]bool
}

func (m *MockTunnel) OnNodeNotReady(ctx context.Context, nodeID string) {
	m.NodeNotReady[nodeID] = true
	return
}

func (m *MockTunnel) PutNode(ctx context.Context, nodeID string, node Node) {
	m.Lock()
	defer m.Unlock()

	m.nodeStorage[nodeID] = node
	m.OnBaseDiscovered(nodeID, node.NodeInfo)
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
	return nil
}

func (m *MockTunnel) Ready() bool {
	return true
}

func (m *MockTunnel) RegisterCallback(
	discovered OnBaseDiscovered,
	onNodeStatusDataArrived OnBaseStatusArrived,
	onAllBizStatusArrived OnAllBizStatusArrived,
	onSingleBizStatusArrived OnSingleBizStatusArrived) {
	m.OnBaseStatusArrived = onNodeStatusDataArrived
	m.OnBaseDiscovered = discovered
	m.OnAllBizStatusArrived = onAllBizStatusArrived
	m.OnSingleBizStatusArrived = onSingleBizStatusArrived
}

func (m *MockTunnel) RegisterNode(ctx context.Context, nodeID string, initData model.NodeInfo) {
	return
}

func (m *MockTunnel) UnRegisterNode(ctx context.Context, nodeID string) {
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

func convertContainerMap2ContainerList(containerMap map[string]model.BizStatusData) []model.BizStatusData {
	ret := make([]model.BizStatusData, 0)
	for _, container := range containerMap {
		ret = append(ret, container)
	}
	return ret
}
