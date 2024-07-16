package test

import (
	"encoding/json"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/java/model"
	"github.com/sirupsen/logrus"
	"strings"
	"sync"
	"time"
)

type BaseMock struct {
	sync.Mutex

	baseID     string
	mqttClient *mqtt.Client
	bizInfos   []ark.ArkBizInfo
	healthData ark.HealthData

	exit chan struct{}
}

func NewBaseMock(baseID, baseName, baseVersion string, mqttClient *mqtt.Client) *BaseMock {
	return &BaseMock{
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

func (bm *BaseMock) Exit() {
	select {
	case <-bm.exit:
	default:
		close(bm.exit)
	}
}

func (bm *BaseMock) Run() {
	commandTopic := fmt.Sprintf("koupleless/%s/+", bm.baseID)
	bm.mqttClient.Sub(commandTopic, 1, bm.commandCallback)
	defer bm.mqttClient.UnSub(commandTopic)
	go func() {
		ticker := time.NewTicker(time.Second * 5)
		for {
			select {
			case <-bm.exit:
				return
			case <-ticker.C:
				masterInfoBytes, _ := json.Marshal(bm.healthData.MasterBizInfo)
				bm.mqttClient.Pub(fmt.Sprintf("koupleless/%s/base/heart", bm.baseID), 0, masterInfoBytes)
			}
		}
	}()
	<-bm.exit
}

func (bm *BaseMock) commandCallback(client paho.Client, msg paho.Message) {
	topic := msg.Topic()
	logrus.Info("reach message from: ", topic)
	fields := strings.Split(topic, "/")
	switch fields[len(fields)-1] {
	case model.CommandHealth:
		healthBytes, _ := json.Marshal(ark.HealthResponse{
			GenericArkResponseBase: ark.GenericArkResponseBase[ark.HealthInfo]{
				Code: "SUCCESS",
				Data: ark.HealthInfo{
					HealthData: bm.healthData,
				},
				Message: "",
			},
		})
		bm.mqttClient.Pub(fmt.Sprintf("koupleless/%s/base/health", bm.baseID), 0, healthBytes)
	case model.CommandQueryAllBiz:
		bizBytes, _ := json.Marshal(ark.QueryAllArkBizResponse{
			GenericArkResponseBase: ark.GenericArkResponseBase[[]ark.ArkBizInfo]{
				Code:    "SUCCESS",
				Data:    bm.bizInfos,
				Message: "",
			},
		})
		bm.mqttClient.Pub(fmt.Sprintf("koupleless/%s/base/biz", bm.baseID), 0, bizBytes)
	case model.CommandInstallBiz:
		var data ark.BizModel
		json.Unmarshal(msg.Payload(), &data)
		// install biz
		bm.Lock()
		installed := false
		for _, bizInfo := range bm.bizInfos {
			if bizInfo.BizName == data.BizName && bizInfo.BizVersion == data.BizVersion {
				installed = true
				break
			}
		}
		if !installed {
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
		}
		bm.Unlock()

		bizBytes, _ := json.Marshal(ark.QueryAllArkBizResponse{
			GenericArkResponseBase: ark.GenericArkResponseBase[[]ark.ArkBizInfo]{
				Code: "SUCCESS",
				Data: bm.bizInfos,
			},
		})
		bm.mqttClient.Pub(fmt.Sprintf("koupleless/%s/base/biz", bm.baseID), 0, bizBytes)
	case model.CommandUnInstallBiz:
		var data ark.BizModel
		json.Unmarshal(msg.Payload(), &data)
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

		bizBytes, _ := json.Marshal(ark.QueryAllArkBizResponse{
			GenericArkResponseBase: ark.GenericArkResponseBase[[]ark.ArkBizInfo]{
				Code: "SUCCESS",
				Data: bm.bizInfos,
			},
		})
		bm.mqttClient.Pub(fmt.Sprintf("koupleless/%s/base/biz", bm.baseID), 0, bizBytes)
	}
}
