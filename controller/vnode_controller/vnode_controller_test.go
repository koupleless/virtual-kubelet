package vnode_controller

import (
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewVNodeController_NoConfig(t *testing.T) {
	_, err := NewVNodeController(nil, nil)
	assert.NotNil(t, err)
}

func TestNewVNodeController_ConfigNoTunnels(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{}, nil)
	assert.NotNil(t, err)
}

func TestNewVNodeController_ConfigNoIdentity(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{}, []tunnel.Tunnel{
		&tunnel.MockTunnel{},
	})
	assert.NotNil(t, err)
}

func TestNewVNodeController_Success(t *testing.T) {
	_, err := NewVNodeController(&model.BuildVNodeControllerConfig{
		VPodIdentity: "suite",
	}, []tunnel.Tunnel{
		&tunnel.MockTunnel{},
	})
	assert.Nil(t, err)
}
