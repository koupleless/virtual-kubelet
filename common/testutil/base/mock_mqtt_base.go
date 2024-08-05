package base

import (
	"encoding/json"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
	"time"
)

type MockMqttBase struct {
	sync.Mutex

	baseID     string
	mqttClient *mqtt.Client
	bizInfos   []ark.ArkBizInfo
	healthData ark.HealthData

	exit chan struct{}
}

func NewBaseMock(baseID, baseName, baseVersion string, mqttClient *mqtt.Client) *MockMqttBase {
	return &MockMqttBase{
		Mutex:      sync.Mutex{},
		baseID:     baseID,
		mqttClient: mqttClient,
		bizInfos:   []ark.ArkBizInfo{},
		healthData: ark.HealthData{
			Jvm: ark.JvmInfo{
				JavaUsedMetaspace:      10000000,
				JavaCommittedMetaspace: 12000000,
				JavaMaxMetaspace:       100000000,
			},
			Cpu: ark.CpuInfo{},
			MasterBizInfo: ark.MasterBizInfo{
				BizName:        baseName,
				BizState:       "ACTIVATED",
				BizVersion:     baseVersion,
				WebContextPath: "/",
			},
		},
		exit: make(chan struct{}),
	}
}

func (bm *MockMqttBase) Exit() {
	select {
	case <-bm.exit:
	default:
		close(bm.exit)
	}
}

func (bm *MockMqttBase) Run() {
	commandTopic := fmt.Sprintf("koupleless_test/%s/+", bm.baseID)
	err := bm.mqttClient.Sub(commandTopic, mqtt.Qos1, bm.commandCallback)
	if err != nil {
		logrus.Error(err.Error())
	}
	defer bm.mqttClient.UnSub(commandTopic)
	go func() {
		ticker := time.NewTicker(time.Minute * 2)
		mqttResponse := map[string]interface{}{
			"publishTimestamp": time.Now().UnixMilli(),
			"data": model.HeartBeatData{
				MasterBizInfo: bm.healthData.MasterBizInfo,
				NetworkInfo:   model.NetworkInfo{LocalIP: "127.0.0.1", LocalHostName: "test_base"},
			},
		}
		heartInfoBytes, _ := json.Marshal(mqttResponse)

		bm.mqttClient.Pub(fmt.Sprintf("koupleless_test/%s/base/heart", bm.baseID), mqtt.Qos1, heartInfoBytes)
		for {
			select {
			case <-bm.exit:
				return
			case <-ticker.C:
				mqttResponse = map[string]interface{}{
					"publishTimestamp": time.Now().UnixMilli(),
					"data": model.HeartBeatData{
						MasterBizInfo: bm.healthData.MasterBizInfo,
						NetworkInfo:   model.NetworkInfo{LocalIP: "127.0.0.1", LocalHostName: "test_base"},
					},
				}
				heartInfoBytes, _ = json.Marshal(mqttResponse)

				bm.mqttClient.Pub(fmt.Sprintf("koupleless_test/%s/base/heart", bm.baseID), mqtt.Qos1, heartInfoBytes)
			}
		}
	}()
	<-bm.exit
	bm.healthData.MasterBizInfo.BizState = "DEACTIVATED"
	mqttResponse := map[string]interface{}{
		"publishTimestamp": time.Now().UnixMilli(),
		"data": model.HeartBeatData{
			MasterBizInfo: bm.healthData.MasterBizInfo,
			NetworkInfo:   model.NetworkInfo{LocalIP: "127.0.0.1", LocalHostName: "test_base"},
		},
	}
	heartInfoBytes, _ := json.Marshal(mqttResponse)

	bm.mqttClient.Pub(fmt.Sprintf("koupleless_test/%s/base/heart", bm.baseID), mqtt.Qos1, heartInfoBytes)
}

func (bm *MockMqttBase) commandCallback(_ paho.Client, msg paho.Message) {
	topic := msg.Topic()
	logrus.Info("reach message from: ", topic)
	fields := strings.Split(topic, "/")
	switch fields[len(fields)-1] {
	case model.CommandHealth:
		response := ark.HealthResponse{
			GenericArkResponseBase: ark.GenericArkResponseBase[ark.HealthInfo]{
				Code: "SUCCESS",
				Data: ark.HealthInfo{
					HealthData: bm.healthData,
				},
				Message: "",
			},
		}
		mqttResponse := map[string]interface{}{
			"publishTimestamp": time.Now().UnixMilli(),
			"data":             response,
		}
		healthBytes, _ := json.Marshal(mqttResponse)
		bm.mqttClient.Pub(fmt.Sprintf("koupleless_test/%s/base/health", bm.baseID), mqtt.Qos1, healthBytes)
	case model.CommandQueryAllBiz:
		response := ark.QueryAllArkBizResponse{
			GenericArkResponseBase: ark.GenericArkResponseBase[[]ark.ArkBizInfo]{
				Code:    "SUCCESS",
				Data:    bm.bizInfos,
				Message: "",
			},
		}
		mqttResponse := map[string]interface{}{
			"publishTimestamp": time.Now().UnixMilli(),
			"data":             response,
		}
		bizBytes, _ := json.Marshal(mqttResponse)

		bm.mqttClient.Pub(fmt.Sprintf("koupleless_test/%s/base/biz", bm.baseID), mqtt.Qos1, bizBytes)
	case model.CommandInstallBiz:
		var data ark.BizModel
		err := json.Unmarshal(msg.Payload(), &data)
		if err != nil {
			logrus.Error(err)
		}
		// install biz
		bm.Lock()
		installedIndex := -1
		diffVersioInstalledIndex := -1
		for index, bizInfo := range bm.bizInfos {
			if bizInfo.BizName == data.BizName && bizInfo.BizVersion == data.BizVersion {
				installedIndex = index
				break
			} else if bizInfo.BizName == data.BizName && bizInfo.BizVersion != data.BizVersion {
				diffVersioInstalledIndex = index
			}
		}
		if diffVersioInstalledIndex != -1 {
			bm.bizInfos[diffVersioInstalledIndex].BizState = "DEACTIVATED"
		}
		if installedIndex == -1 {
			bm.bizInfos = append(bm.bizInfos, ark.ArkBizInfo{
				BizName:    data.BizName,
				BizState:   "ACTIVATED",
				BizVersion: data.BizVersion,
				BizStateRecords: []ark.ArkBizStateRecord{
					{
						ChangeTime: "2024-07-09 16:48:56.921",
						State:      "ACTIVATED",
						Reason:     "installed",
						Message:    "installed successfully",
					},
				},
			})
		} else {
			bm.bizInfos[installedIndex].BizState = "ACTIVATED"
		}
		bm.Unlock()

		response := ark.QueryAllArkBizResponse{
			GenericArkResponseBase: ark.GenericArkResponseBase[[]ark.ArkBizInfo]{
				Code:    "SUCCESS",
				Data:    bm.bizInfos,
				Message: "",
			},
		}
		mqttResponse := map[string]interface{}{
			"publishTimestamp": time.Now().UnixMilli(),
			"data":             response,
		}
		bizBytes, _ := json.Marshal(mqttResponse)
		bm.mqttClient.Pub(fmt.Sprintf("koupleless_test/%s/base/biz", bm.baseID), mqtt.Qos1, bizBytes)
	case model.CommandUnInstallBiz:
		var data ark.BizModel
		err := json.Unmarshal(msg.Payload(), &data)
		if err != nil {
			logrus.Error(err)
		}
		bm.Lock()
		index := -1
		for i, bizInfo := range bm.bizInfos {
			if bizInfo.BizName == data.BizName && bizInfo.BizVersion == data.BizVersion {
				index = i
				break
			}
		}
		if index != -1 {
			bm.bizInfos = append(bm.bizInfos[:index], bm.bizInfos[index+1:]...)
		}
		bm.Unlock()

		response := ark.QueryAllArkBizResponse{
			GenericArkResponseBase: ark.GenericArkResponseBase[[]ark.ArkBizInfo]{
				Code:    "SUCCESS",
				Data:    bm.bizInfos,
				Message: "",
			},
		}
		mqttResponse := map[string]interface{}{
			"publishTimestamp": time.Now().UnixMilli(),
			"data":             response,
		}
		bizBytes, _ := json.Marshal(mqttResponse)
		bm.mqttClient.Pub(fmt.Sprintf("koupleless_test/%s/base/biz", bm.baseID), mqtt.Qos1, bizBytes)
	}
}
