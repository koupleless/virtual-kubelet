package base_pod

import "encoding/json"

type PatchOpType string

type PatchOps []PatchOp

type PatchOp struct {
	OP    PatchOpType `json:"op"`
	Path  string      `json:"path,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

func (ops *PatchOps) bytes() []byte {
	opsBytes, _ := json.Marshal(ops)
	return opsBytes
}
