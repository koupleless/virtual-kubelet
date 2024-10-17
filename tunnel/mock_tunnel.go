package tunnel

import (
	"context"
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
	OnNodeDiscovered
	OnNodeStatusDataArrived
	OnSingleContainerStatusChanged
	OnQueryAllContainerStatusDataArrived

	containerStorage map[string]map[string]model.ContainerStatusData
	nodeStorage      map[string]Node
	NodeNotReady     map[string]bool
}

func (m *MockTunnel) OnNodeNotReady(ctx context.Context, info model.UnreachableNodeInfo) {
	m.NodeNotReady[info.NodeID] = true
	return
}

func (m *MockTunnel) PutNode(ctx context.Context, nodeID string, node Node) {
	m.Lock()
	defer m.Unlock()

	m.nodeStorage[nodeID] = node
	m.OnNodeDiscovered(nodeID, node.NodeInfo, m)
}

func (m *MockTunnel) DeleteNode(nodeID string) {
	m.Lock()
	defer m.Unlock()

	delete(m.nodeStorage, nodeID)
}

func (m *MockTunnel) PutContainer(ctx context.Context, nodeID, containerKey string, data model.ContainerStatusData) {
	m.Lock()
	defer m.Unlock()

	containerMap, has := m.containerStorage[nodeID]
	if !has {
		containerMap = map[string]model.ContainerStatusData{}
	}
	containerMap[containerKey] = data
	m.containerStorage[nodeID] = containerMap
	m.OnSingleContainerStatusChanged(nodeID, data)
}

func (m *MockTunnel) Key() string {
	return "mock_tunnel"
}

func (m *MockTunnel) Start(ctx context.Context, clientID string, env string) error {
	m.containerStorage = map[string]map[string]model.ContainerStatusData{}
	m.nodeStorage = map[string]Node{}
	m.NodeNotReady = map[string]bool{}
	return nil
}

func (m *MockTunnel) Ready() bool {
	return true
}

func (m *MockTunnel) RegisterCallback(
	discovered OnNodeDiscovered,
	onNodeStatusDataArrived OnNodeStatusDataArrived,
	onQueryAllContainerStatusDataArrived OnQueryAllContainerStatusDataArrived,
	onSingleContainerStatusChanged OnSingleContainerStatusChanged) {
	m.OnNodeStatusDataArrived = onNodeStatusDataArrived
	m.OnNodeDiscovered = discovered
	m.OnQueryAllContainerStatusDataArrived = onQueryAllContainerStatusDataArrived
	m.OnSingleContainerStatusChanged = onSingleContainerStatusChanged
}

func (m *MockTunnel) OnNodeStart(ctx context.Context, nodeID string) {
	return
}

func (m *MockTunnel) OnNodeStop(ctx context.Context, nodeID string) {
	return
}

func (m *MockTunnel) FetchHealthData(ctx context.Context, nodeID string) error {
	data, has := m.nodeStorage[nodeID]
	if has {
		m.OnNodeStatusDataArrived(nodeID, data.NodeStatusData)
	}
	return nil
}

func (m *MockTunnel) QueryAllContainerStatusData(ctx context.Context, nodeID string) error {
	_, has := m.nodeStorage[nodeID]
	if has {
		m.OnQueryAllContainerStatusDataArrived(nodeID, translateContainerMap2ContainerList(m.containerStorage[nodeID]))
	}
	return nil
}

func (m *MockTunnel) StartContainer(ctx context.Context, nodeID, podKey string, container *corev1.Container) error {
	m.Lock()
	defer m.Unlock()
	key := m.GetContainerUniqueKey(podKey, container)
	containerMap, has := m.containerStorage[nodeID]
	if !has {
		containerMap = map[string]model.ContainerStatusData{}
	}
	data := model.ContainerStatusData{
		Key:        key,
		Name:       container.Name,
		PodKey:     podKey,
		State:      model.ContainerStateResolved,
		ChangeTime: time.Now(),
		Reason:     "mock_resolved",
		Message:    "mock resolved",
	}
	containerMap[key] = data

	m.containerStorage[nodeID] = containerMap
	m.OnSingleContainerStatusChanged(nodeID, data)
	return nil
}

func (m *MockTunnel) ShutdownContainer(ctx context.Context, nodeID, podKey string, container *corev1.Container) error {
	m.Lock()
	defer m.Unlock()
	containerMap := m.containerStorage[nodeID]
	key := m.GetContainerUniqueKey(podKey, container)
	data := containerMap[key]
	delete(containerMap, key)
	m.containerStorage[nodeID] = containerMap
	data.State = model.ContainerStateDeactivated
	m.OnSingleContainerStatusChanged(nodeID, data)
	return nil
}

func (m *MockTunnel) GetContainerUniqueKey(podKey string, container *corev1.Container) string {
	return podKey + container.Name
}

func translateContainerMap2ContainerList(containerMap map[string]model.ContainerStatusData) []model.ContainerStatusData {
	ret := make([]model.ContainerStatusData, 0)
	for _, container := range containerMap {
		ret = append(ret, container)
	}
	return ret
}
