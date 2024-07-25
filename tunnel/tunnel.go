package tunnel

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/model"
)

type BaseDiscoveredCallback func(string, model.HeartBeatData, Tunnel)

type HealthDataArrivedCallback func(string, ark.HealthData)

type QueryAllBizDataArrivedCallback func(string, []ark.ArkBizInfo)

type Tunnel interface {
	Name() string

	Register(context.Context, string, BaseDiscoveredCallback, HealthDataArrivedCallback, QueryAllBizDataArrivedCallback) error

	FetchHealthData(context.Context, string) error

	QueryAllBizData(context.Context, string) error

	InstallBiz(context.Context, string, *ark.BizModel) error

	UninstallBiz(context.Context, string, *ark.BizModel) error
}
