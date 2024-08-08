package tunnel

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/model"
)

type OnBaseDiscovered func(string, model.HeartBeatData, Tunnel)

type OnHealthDataArrived func(string, ark.HealthData)

type OnQueryAllBizDataArrived func(string, []ark.ArkBizInfo)

type OnInstallBizResponseArrived func(string, ark.InstallBizResponse)

type OnUnInstallBizResponseArrived func(string, ark.UnInstallBizResponse)

type Tunnel interface {
	Name() string

	Register(context.Context, string, string, OnBaseDiscovered, OnHealthDataArrived, OnQueryAllBizDataArrived, OnInstallBizResponseArrived, OnUnInstallBizResponseArrived) error

	OnBaseStart(context.Context, string)

	OnBaseStop(context.Context, string)

	FetchHealthData(context.Context, string) error

	QueryAllBizData(context.Context, string) error

	InstallBiz(context.Context, string, *ark.BizModel) error

	UninstallBiz(context.Context, string, *ark.BizModel) error
}
