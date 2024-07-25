package model

import "github.com/koupleless/arkctl/v1/service/ark"

// HeartBeatData is the data of base heart beat.
type HeartBeatData struct {
	DeviceID      string            `json:"deviceID"`
	MasterBizInfo ark.MasterBizInfo `json:"masterBizInfo"`
	NetworkInfo   struct {
		LocalIP       string `json:"localIP"`
		LocalHostName string `json:"localHostName"`
		ArkletPort    int    `json:"arkPort"`
	} `json:"networkInfo"`
}
