package koupleless_mqtt_tunnel

const (
	CommandHealth       = "health"
	CommandQueryAllBiz  = "queryAllBiz"
	CommandInstallBiz   = "installBiz"
	CommandUnInstallBiz = "uninstallBiz"
)

const (
	BaseHeartBeatTopic            = "koupleless_%s/+/base/heart"
	BaseQueryBaselineTopic        = "koupleless_%s/+/base/queryBaseline"
	BaseHealthTopic               = "koupleless_%s/%s/base/health"
	BaseBizTopic                  = "koupleless_%s/%s/base/biz"
	BaseBizOperationResponseTopic = "koupleless_%s/%s/base/bizOperation"
	BaseBaselineResponseTopic     = "koupleless_%s/%s/base/baseline"
)

const (
	LabelKeyOfTechStack = "base.koupleless.io/stack"
)
