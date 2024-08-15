package mqtt_tunnel

import (
	"fmt"
	"github.com/koupleless/arkctl/common/fileutil"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"strings"
	"time"
)

func getBaseIDFromTopic(topic string) string {
	fileds := strings.Split(topic, "/")
	if len(fileds) < 2 {
		return ""
	}
	return fileds[1]
}

func expired(publishTimestamp int64, maxLiveMilliSec int64) bool {
	return publishTimestamp+maxLiveMilliSec <= time.Now().UnixMilli()
}

func formatArkletCommandTopic(env, baseID, command string) string {
	return fmt.Sprintf("koupleless_%s/%s/%s", env, baseID, command)
}

func formatBaselineResponseTopic(env, baseID string) string {
	return fmt.Sprintf(BaseBaselineResponseTopic, env, baseID)
}

func getBizVersionFromContainer(container *corev1.Container) string {
	bizVersion := ""
	for _, env := range container.Env {
		if env.Name == "BIZ_VERSION" {
			bizVersion = env.Value
			break
		}
	}
	return bizVersion
}

func translateCoreV1ContainerToBizModel(container *corev1.Container) ark.BizModel {
	return ark.BizModel{
		BizName:    container.Name,
		BizVersion: getBizVersionFromContainer(container),
		BizUrl:     fileutil.FileUrl(container.Image),
	}
}

func getBizIdentity(bizName, bizVersion string) string {
	return bizName + ":" + bizVersion
}

func translateHeartBeatDataToNodeInfo(data HeartBeatData) model.NodeInfo {
	state := model.NodeStatusDeactivated
	if data.MasterBizInfo.BizState == "ACTIVATED" {
		state = model.NodeStatusActivated
	}
	return model.NodeInfo{
		Metadata: model.NodeMetadata{
			Name:    data.MasterBizInfo.BizName,
			Version: data.MasterBizInfo.BizVersion,
			Status:  state,
		},
		NetworkInfo: model.NetworkInfo{
			NodeIP:   data.NetworkInfo.LocalIP,
			HostName: data.NetworkInfo.LocalHostName,
		},
	}
}

func translateHealthDataToNodeStatus(data ark.HealthData) model.NodeStatusData {
	resourceMap := make(map[corev1.ResourceName]model.NodeResource)
	memory := model.NodeResource{}
	if data.Jvm.JavaMaxMetaspace != -1 {
		memory.Capacity = utils.ConvertByteNumToResourceQuantity(data.Jvm.JavaMaxMetaspace)
	}
	if data.Jvm.JavaCommittedMetaspace != -1 {
		memory.Allocatable = utils.ConvertByteNumToResourceQuantity(data.Jvm.JavaCommittedMetaspace)
	}
	resourceMap[corev1.ResourceMemory] = memory
	return model.NodeStatusData{
		Resources: resourceMap,
		CustomLabels: map[string]string{
			LabelKeyOfTechStack: "java",
		},
	}
}

func translateHeartBeatDataToBaselineQuery(data ark.MasterBizInfo) model.QueryBaselineRequest {
	return model.QueryBaselineRequest{
		Name:    data.BizName,
		Version: data.BizVersion,
		CustomLabels: map[string]string{
			LabelKeyOfTechStack: "java",
		},
	}
}

func translateQueryAllBizDataToContainerStatuses(data []ark.ArkBizInfo) []model.ContainerStatusData {
	ret := make([]model.ContainerStatusData, 0)
	for _, bizInfo := range data {
		changeTime, reason, message := getLatestState(bizInfo.BizState, bizInfo.BizStateRecords)
		ret = append(ret, model.ContainerStatusData{
			Key:        getBizIdentity(bizInfo.BizName, bizInfo.BizVersion),
			Name:       bizInfo.BizName,
			PodKey:     model.PodKeyAll,
			State:      getContainerStateFromBizState(bizInfo.BizState),
			ChangeTime: changeTime,
			Reason:     reason,
			Message:    message,
		})
	}
	return ret
}

func translateCommandResponseToContainerOperationResponseData(data BizOperationResponse) model.ContainerOperationResponseData {
	result := model.OperationResponseCodeSuccess
	// uninstall has a special error code called NOT_FOUND_BIZ, it means biz not installed, we should mark this kind of uninstall response success
	if data.Response.Code != "SUCCESS" && (data.Command == CommandUnInstallBiz && data.Response.Data.Code != "NOT_FOUND_BIZ") {
		result = model.OperationResponseCodeFailure
	}
	return model.ContainerOperationResponseData{
		ContainerKey: getBizIdentity(data.BizName, data.BizVersion),
		Result:       result,
		Reason:       data.Response.Message,
		Message:      data.Response.ErrorStackTrace,
	}
}

func getContainerStateFromBizState(bizState string) model.ContainerState {
	switch bizState {
	case "ACTIVATED":
		return model.ContainerStateActivated
	case "DEACTIVATED":
		return model.ContainerStateDeactivated
	}
	return model.ContainerStateResolved
}

func getLatestState(state string, records []ark.ArkBizStateRecord) (time.Time, string, string) {
	latestStateTime := time.UnixMilli(0)
	reason := ""
	message := ""
	for _, record := range records {
		if record.State != state {
			continue
		}
		if len(record.ChangeTime) < 3 {
			continue
		}
		changeTime, err := time.Parse("2006-01-02 15:04:05", record.ChangeTime[:len(record.ChangeTime)-3])
		if err != nil {
			logrus.Errorf("failed to parse change time %s", record.ChangeTime)
			continue
		}
		if changeTime.UnixMilli() > latestStateTime.UnixMilli() {
			latestStateTime = changeTime
			reason = record.Reason
			message = record.Message
		}
	}
	return latestStateTime, reason, message
}
