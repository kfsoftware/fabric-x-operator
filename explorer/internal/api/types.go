package api

import (
	"github.com/kfsoftware/fabric-x-operator/explorer/internal/decoder"
)

type DashboardResponse struct {
	Height       uint64                    `json:"height"`
	RecentBlocks []decoder.DecodedBlock    `json:"recentBlocks"`
}

type BlockListResponse struct {
	Blocks []decoder.DecodedBlock `json:"blocks"`
	Total  uint64                 `json:"total"`
}

type NamespacesResponse struct {
	Namespaces []string `json:"namespaces"`
}

type StateResponse struct {
	Namespace string     `json:"namespace"`
	Rows      []StateRow `json:"rows"`
}

type StateRow struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version uint64 `json:"version"`
}

type TxStatusResponse struct {
	TxID   string `json:"txId"`
	Status string `json:"status"`
	Code   int32  `json:"code"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
