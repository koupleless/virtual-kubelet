package mqtt_tunnel

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/common/testutil/mqtt_client"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func init() {
	mqtt.DefaultMqttClientInitFunc = mqtt_client.NewMockMqttClient
}

var currBaseID string

var currBaseHealthFetched string

var currBaseBizFetched string

var currBizInfoFetched []ark.ArkBizInfo

func baseDiscoverCallback(baseID string, data model.HeartBeatData, t tunnel.Tunnel) {
	logrus.Info("Discovered base id:", baseID)
	currBaseID = baseID
}

func healthDataCallback(s string, data ark.HealthData) {
	logrus.Info("Health data received: ", s)
	currBaseHealthFetched = s
}

func bizDataCallback(s string, infos []ark.ArkBizInfo) {
	logrus.Info("Biz data received:", s)
	currBaseBizFetched = s
	currBizInfoFetched = infos
}

func TestMqttTunnel_Register(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", baseDiscoverCallback, healthDataCallback, bizDataCallback, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, "mqtt_tunnel_provider", mt.Name())
}

func TestMqttTunnel_BaseDiscover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", baseDiscoverCallback, healthDataCallback, bizDataCallback, nil, nil)
	assert.NoError(t, err)

	// mock base online
	id := "test-base-discover"

	mt.OnBaseStart(ctx, id)

	mt.onBaseDiscovered(id, model.HeartBeatData{}, &mt)

	mt.OnBaseStop(ctx, id)

	assert.Eventually(t, func() bool {
		return currBaseID == id
	}, time.Second*5, 100*time.Millisecond)
}

func TestMqttTunnel_FetchBaseInfo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", baseDiscoverCallback, healthDataCallback, bizDataCallback, nil, nil)
	assert.NoError(t, err)

	// mock base online
	id := "test-base-fetch"

	mt.onBaseDiscovered(id, model.HeartBeatData{}, &mt)

	err = mt.FetchHealthData(ctx, id)
	assert.NoError(t, err)

	mt.onHealthDataArrived(id, ark.HealthData{})

	err = mt.QueryAllBizData(ctx, id)
	assert.NoError(t, err)

	mt.onQueryAllBizDataArrived(id, []ark.ArkBizInfo{})

	assert.Eventually(t, func() bool {
		return currBaseID == id && currBaseHealthFetched == id && currBaseBizFetched == id
	}, time.Second*5, 100*time.Millisecond)
}

func TestMqttTunnel_BaseBizOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", baseDiscoverCallback, healthDataCallback, bizDataCallback, nil, nil)
	assert.NoError(t, err)

	// mock base online
	id := "test-base-biz-operation"

	mt.onBaseDiscovered(id, model.HeartBeatData{}, &mt)

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

	mt.onQueryAllBizDataArrived(id, []ark.ArkBizInfo{
		{
			BizName:    bizModel.BizName,
			BizVersion: bizModel.BizVersion,
			BizState:   "ACTIVATED",
		},
	})

	assert.Eventually(t, func() bool {
		return len(currBizInfoFetched) == 1 && currBizInfoFetched[0].BizName == bizModel.BizName
	}, time.Second*5, 100*time.Millisecond)

	err = mt.UninstallBiz(ctx, id, &bizModel)
	assert.NoError(t, err)

	mt.onQueryAllBizDataArrived(id, []ark.ArkBizInfo{})

	assert.Eventually(t, func() bool {
		return len(currBizInfoFetched) == 0
	}, time.Second*5, 100*time.Millisecond)
}

func TestMqttTunnel_MqttHeartCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", baseDiscoverCallback, healthDataCallback, bizDataCallback, nil, nil)
	assert.NoError(t, err)

	// mock base online
	id := "test-base-mqtt-heart-callback"

	// error msg
	mt.heartBeatMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/heart",
		P: []byte(""),
	})

	// expired msg
	mt.heartBeatMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/heart",
		P: []byte("{}"),
	})

	// valid msg
	mt.heartBeatMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/heart",
		P: []byte("{\"publishTimestamp\":1000000000000000}"),
	})

	assert.Eventually(t, func() bool {
		return currBaseID == id
	}, time.Second*5, 100*time.Millisecond)
}

func TestMqttTunnel_MqttHealthCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", baseDiscoverCallback, healthDataCallback, bizDataCallback, nil, nil)
	assert.NoError(t, err)

	// mock base online
	id := "test-base-mqtt-health-callback"

	// error msg
	mt.healthMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/health",
		P: []byte(""),
	})

	// expired msg
	mt.healthMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/health",
		P: []byte("{}"),
	})

	// not success
	mt.healthMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/health",
		P: []byte("{\"publishTimestamp\":1000000000000000}"),
	})

	// valid msg
	mt.healthMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/health",
		P: []byte("{\"publishTimestamp\":1000000000000000, \"data\":{\"code\":\"SUCCESS\"}}"),
	})

	assert.Eventually(t, func() bool {
		return currBaseHealthFetched == id
	}, time.Second*5, 100*time.Millisecond)
}

func TestMqttTunnel_MqttBizCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mt := MqttTunnel{}

	err := mt.Register(ctx, "test-client", "test", baseDiscoverCallback, healthDataCallback, bizDataCallback, nil, nil)
	assert.NoError(t, err)

	// mock base online
	id := "test-base-mqtt-biz-callback"

	// error msg
	mt.bizMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/biz",
		P: []byte(""),
	})

	// expired msg
	mt.bizMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/biz",
		P: []byte("{}"),
	})

	// not success
	mt.bizMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/biz",
		P: []byte("{\"publishTimestamp\":1000000000000000}"),
	})

	// valid msg
	mt.bizMsgCallback(nil, &mqtt_client.MockMessage{
		T: "koupleless_test/" + id + "/base/biz",
		P: []byte("{\"publishTimestamp\":1000000000000000, \"data\":{\"code\":\"SUCCESS\"}}"),
	})

	assert.Eventually(t, func() bool {
		return currBaseBizFetched == id
	}, time.Second*5, 100*time.Millisecond)
}
