package mqtt_tunnel

import (
	"context"
	"fmt"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/common/testutil/base"
	"github.com/koupleless/virtual-kubelet/common/testutil/mqtt_broker"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var currBaseID string

var currBaseHealthFetched string

var currBaseBizFetched string

var currBizInfoFetched []ark.ArkBizInfo

func baseDiscoverCallback(baseID string, data model.HeartBeatData, t tunnel.Tunnel) {
	log.G(context.TODO()).Debugf("Discovered base id: %s", baseID)
	currBaseID = baseID
}

func healthDataCallback(s string, data ark.HealthData) {
	log.G(context.TODO()).Debugf("Health data received: %s", s)
	currBaseHealthFetched = s
}

func bizDataCallback(s string, infos []ark.ArkBizInfo) {
	log.G(context.TODO()).Debugf("Biz data received: %s", s)
	currBaseBizFetched = s
	currBizInfoFetched = infos
}

func TestMqttTunnel_Register(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mqtt_broker.StartLocalMqttBroker()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", baseDiscoverCallback, healthDataCallback, bizDataCallback)
	assert.NoError(t, err)
	assert.Equal(t, "mqtt_tunnel_provider", mt.Name())
}

func TestMqttTunnel_BaseDiscover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mqtt_broker.StartLocalMqttBroker()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", baseDiscoverCallback, healthDataCallback, bizDataCallback)
	assert.NoError(t, err)

	client, err := mqtt.NewMqttClient(&mqtt.ClientConfig{
		Broker:   "localhost",
		Port:     1883,
		ClientID: "TestNewMqttClientID",
		Username: "local",
		Password: "public",
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	// mock base online
	id := "test-base-discover"
	mockBase := base.NewBaseMock(id, "base", "1.0.0", client)

	go mockBase.Run()
	assert.Eventually(t, func() bool {
		return currBaseID == id
	}, time.Second*5, 100*time.Millisecond)
}

func TestMqttTunnel_FetchBaseInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mqtt_broker.StartLocalMqttBroker()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", baseDiscoverCallback, healthDataCallback, bizDataCallback)
	assert.NoError(t, err)

	client, err := mqtt.NewMqttClient(&mqtt.ClientConfig{
		Broker:   "localhost",
		Port:     1883,
		ClientID: "TestNewMqttClientID",
		Username: "local",
		Password: "public",
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	// mock base online
	id := "test-base-fetch"
	mockBase := base.NewBaseMock(id, "base", "1.0.0", client)

	go mockBase.Run()

	err = mt.FetchHealthData(ctx, id)
	assert.NoError(t, err)

	err = mt.QueryAllBizData(ctx, id)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool {
		return currBaseID == id && currBaseHealthFetched == id && currBaseBizFetched == id
	}, time.Second*5, 100*time.Millisecond)
}

func TestMqttTunnel_BaseBizOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mqtt_broker.StartLocalMqttBroker()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", baseDiscoverCallback, healthDataCallback, bizDataCallback)
	assert.NoError(t, err)

	client, err := mqtt.NewMqttClient(&mqtt.ClientConfig{
		Broker:   "localhost",
		Port:     1883,
		ClientID: "TestNewMqttClientID",
		Username: "local",
		Password: "public",
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	// mock base online
	id := "test-base-biz-operation"
	mockBase := base.NewBaseMock(id, "base", "1.0.0", client)

	go mockBase.Run()

	assert.Eventually(t, func() bool {
		return currBaseID == id
	}, time.Second*5, 100*time.Millisecond)

	bizModel := ark.BizModel{
		BizName:    "test",
		BizVersion: "0.0.1",
		BizUrl:     "test-url",
	}
	err = mt.InstallBiz(ctx, id, &bizModel)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool {
		return len(currBizInfoFetched) == 1 && currBizInfoFetched[0].BizName == bizModel.BizName
	}, time.Second*5, 100*time.Millisecond)

	err = mt.UninstallBiz(ctx, id, &bizModel)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool {
		return len(currBizInfoFetched) == 0
	}, time.Second*5, 100*time.Millisecond)
}
