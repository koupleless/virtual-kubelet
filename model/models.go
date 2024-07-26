package model

import "github.com/koupleless/arkctl/v1/service/ark"

type NetworkInfo struct {
	LocalIP       string `json:"localIP"`
	LocalHostName string `json:"localHostName"`
	ArkletPort    int    `json:"arkletPort"`
}

// HeartBeatData is the data of base heart beat.
type HeartBeatData struct {
	DeviceID      string            `json:"deviceID"`
	MasterBizInfo ark.MasterBizInfo `json:"masterBizInfo"`
	NetworkInfo   NetworkInfo       `json:"networkInfo"`
}
