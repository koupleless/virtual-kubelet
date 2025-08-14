package tunnel

import (
	"context"
	"sync"
	"time"

	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	corev1 "k8s.io/api/core/v1"
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

func (m *MockTunnel) OnNodeNotReady(nodeName string) {
	m.Lock()
	defer m.Unlock()
	delete(m.nodeStorage, nodeName)
	return
}

func (m *MockTunnel) PutNode(ctx context.Context, nodeName string, node Node) {
	m.Lock()
	defer m.Unlock()

	m.nodeStorage[nodeName] = node
	m.OnBaseDiscovered(node.NodeInfo)
	m.OnBaseStatusArrived(nodeName, node.NodeStatusData)
}

func (m *MockTunnel) DeleteNode(nodeName string) {
	m.Lock()
	defer m.Unlock()

	delete(m.nodeStorage, nodeName)
}

func (m *MockTunnel) UpdateBizStatus(nodeName, containerKey string, data model.BizStatusData) {
	m.Lock()
	defer m.Unlock()

	bizStatusMap, has := m.bizStatusStorage[nodeName]
	if !has {
		bizStatusMap = map[string]model.BizStatusData{}
	}
	bizStatusMap[containerKey] = data
	m.bizStatusStorage[nodeName] = bizStatusMap
	m.OnSingleBizStatusArrived(nodeName, data)
}

func (m *MockTunnel) Key() string {
	return "mock_tunnel"
}

func (m *MockTunnel) Start(clientID string, env string) error {
	m.bizStatusStorage = map[string]map[string]model.BizStatusData{}
	m.nodeStorage = map[string]Node{}
	m.NodeNotReady = map[string]bool{}
	return nil
}

func (m *MockTunnel) Ready() bool {
	return true
}

func (m *MockTunnel) RegisterCallback(
	OnBaseDiscovered OnBaseDiscovered,
	OnBaseStatusArrived OnBaseStatusArrived,
	OnAllBizStatusArrived OnAllBizStatusArrived,
	OnSingleBizStatusArrived OnSingleBizStatusArrived) {
	m.OnBaseStatusArrived = OnBaseStatusArrived
	m.OnBaseDiscovered = OnBaseDiscovered
	m.OnAllBizStatusArrived = OnAllBizStatusArrived
	m.OnSingleBizStatusArrived = OnSingleBizStatusArrived
}

func (m *MockTunnel) RegisterNode(initData model.NodeInfo) error {
	return nil
}

func (m *MockTunnel) UnRegisterNode(nodeName string) {
	return
}

func (m *MockTunnel) FetchHealthData(nodeName string) error {
	data, has := m.nodeStorage[nodeName]
	if has {
		m.OnBaseStatusArrived(nodeName, data.NodeStatusData)
	}
	return nil
}

func (m *MockTunnel) QueryAllBizStatusData(nodeName string) error {
	_, has := m.nodeStorage[nodeName]
	if has {
		m.OnAllBizStatusArrived(nodeName, convertContainerMap2ContainerList(m.bizStatusStorage[nodeName]))
	}
	return nil
}

func (m *MockTunnel) StartBiz(nodeName, podKey string, container *corev1.Container) error {
	m.Lock()
	defer m.Unlock()
	key := utils.GetBizUniqueKey(container)
	containerMap, has := m.bizStatusStorage[nodeName]
	if !has {
		containerMap = map[string]model.BizStatusData{}
	}
	data := model.BizStatusData{
		Key:    key,
		Name:   container.Name,
		PodKey: podKey,
		// @fix: should be resolved state when biz start
		State:      string(model.BizStateResolved),
		ChangeTime: time.Now(),
		Reason:     "mock_resolved",
		Message:    "mock resolved",
	}
	containerMap[key] = data

	m.bizStatusStorage[nodeName] = containerMap

	// start to biz installation
	m.OnSingleBizStatusArrived(nodeName, data)
	return nil
}

func (m *MockTunnel) StopBiz(nodeName, podKey string, container *corev1.Container) error {
	m.Lock()
	defer m.Unlock()
	containerMap := m.bizStatusStorage[nodeName]
	key := utils.GetBizUniqueKey(container)
	if data, found := containerMap[key]; found {
		delete(containerMap, key)
		m.bizStatusStorage[nodeName] = containerMap
		data.State = string(model.BizStateStopped)
		data.ChangeTime = time.Now()
		m.OnSingleBizStatusArrived(nodeName, data)
	}
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
