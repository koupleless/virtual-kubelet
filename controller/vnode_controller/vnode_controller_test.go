package vnode_controller

import (
	mockTunnel "github.com/koupleless/virtual-kubelet/common/testutils/tunnel"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewVNodeController_NoConfig(t *testing.T) {
	_, err := NewVNodeController(nil)
	assert.NotNil(t, err)
}

func TestNewVNodeController_ConfigNoTunnels(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{
		Tunnels: nil,
	})
	assert.NotNil(t, err)
}

func TestNewVNodeController_ConfigNoIdentity(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{
		Tunnels: []tunnel.Tunnel{
			&mockTunnel.MockTunnel{},
		},
	})
	assert.NotNil(t, err)
}

func TestNewVNodeController_Success(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{
		Tunnels: []tunnel.Tunnel{
			&mockTunnel.MockTunnel{},
		},
		VPodIdentity: "suite",
	})
	assert.Nil(t, err)
}
