package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/kfsoftware/fabric-x-operator/explorer/internal/decoder"
	"github.com/kfsoftware/fabric-x-operator/explorer/internal/grpcclient"
	"github.com/kfsoftware/fabric-x-operator/explorer/internal/pgstore"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Handlers struct {
	grpc *grpcclient.Client
	pg   *pgstore.Store
}

func NewHandlers(grpc *grpcclient.Client, pg *pgstore.Store) *Handlers {
	return &Handlers{grpc: grpc, pg: pg}
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/dashboard", h.Dashboard)
	mux.HandleFunc("GET /api/v1/transactions", h.TransactionList)
	mux.HandleFunc("GET /api/v1/transactions/{txid}", h.TxDetail)
	mux.HandleFunc("GET /api/v1/blocks/{number}", h.BlockTxs)
	mux.HandleFunc("GET /api/v1/namespaces", h.Namespaces)
	mux.HandleFunc("GET /api/v1/state/{namespace}", h.State)
	mux.HandleFunc("GET /api/v1/events", SSEHandler(h.grpc, h.pg))
	mux.HandleFunc("GET /healthz", h.Healthz)
}

func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	// Get blockchain info from sidecar
	info, err := h.grpc.Blocks.GetBlockchainInfo(r.Context(), &emptypb.Empty{})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	height := info.GetHeight()

	txCount, err := h.pg.GetTransactionCount(r.Context())
	if err != nil {
		log.Printf("get tx count: %v", err)
	}

	recentTxs, err := h.pg.GetTransactions(r.Context(), 10, 0)
	if err != nil {
		log.Printf("get recent txs: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"height":      height,
		"totalBlocks": height,
		"totalTxs":    txCount,
		"recentTxs":   recentTxs,
	})
}

func (h *Handlers) TransactionList(w http.ResponseWriter, r *http.Request) {
	limit := parseInt(r.URL.Query().Get("limit"), 20)
	offset := parseInt(r.URL.Query().Get("offset"), 0)

	txs, err := h.pg.GetTransactions(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	total, _ := h.pg.GetTransactionCount(r.Context())

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"transactions": txs,
		"total":        total,
	})
}

func (h *Handlers) TxDetail(w http.ResponseWriter, r *http.Request) {
	txid := r.PathValue("txid")

	// Get status from DB
	tx, err := h.pg.GetTransactionByID(r.Context(), txid)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	result := map[string]interface{}{
		"txId":       tx.TxID,
		"heightHex":  tx.HeightHex,
		"blockNum":   tx.BlockNum,
		"txNum":      tx.TxNum,
		"status":     tx.Status,
		"statusName": tx.StatusName,
	}

	// Fetch and decode the actual envelope from BlockQueryService (sidecar)
	env, err := h.grpc.Blocks.GetTxByID(r.Context(), &committerpb.TxID{TxId: txid})
	if err != nil {
		log.Printf("GetTxByID %s: %v", txid, err)
	} else {
		decoded, err := decoder.DecodeEnvelope(env)
		if err != nil {
			log.Printf("decode envelope %s: %v", txid, err)
		} else {
			result["channelId"] = decoded.ChannelID
			result["timestamp"] = decoded.Timestamp
			result["type"] = decoded.Type
			result["namespaces"] = decoded.Namespaces
			result["endorsers"] = decoded.Endorsers
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) BlockTxs(w http.ResponseWriter, r *http.Request) {
	numStr := r.PathValue("number")
	num, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid block number"})
		return
	}

	// Try to get the full block from BlockQueryService (sidecar)
	block, err := h.grpc.Blocks.GetBlockByNumber(r.Context(), &committerpb.BlockNumber{Number: num})
	if err != nil {
		log.Printf("GetBlockByNumber %d from sidecar: %v, falling back to DB", num, err)
		// Fallback to DB
		txs, err := h.pg.GetTransactionsByBlock(r.Context(), num)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"blockNumber":  num,
			"transactions": txs,
			"txCount":      len(txs),
		})
		return
	}

	decoded, err := decoder.DecodeBlock(block)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, decoded)
}

func (h *Handlers) Namespaces(w http.ResponseWriter, r *http.Request) {
	// Try gRPC first
	if h.grpc != nil {
		policies, err := h.grpc.Query.GetNamespacePolicies(r.Context(), &emptypb.Empty{})
		if err == nil {
			var names []string
			for _, p := range policies.GetPolicies() {
				names = append(names, p.GetNamespace())
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"namespaces": names})
			return
		}
		log.Printf("gRPC GetNamespacePolicies failed, falling back to DB: %v", err)
	}

	// Fallback to DB
	names, err := h.pg.ListNamespaces(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"namespaces": names})
}

func (h *Handlers) State(w http.ResponseWriter, r *http.Request) {
	ns := r.PathValue("namespace")
	limit := parseInt(r.URL.Query().Get("limit"), 100)

	// Try gRPC first for specific keys
	keysParam := r.URL.Query().Get("keys")
	if keysParam != "" && h.grpc != nil {
		var keys [][]byte
		for _, k := range splitKeys(keysParam) {
			keys = append(keys, []byte(k))
		}
		rows, err := h.grpc.Query.GetRows(r.Context(), &committerpb.Query{
			Namespaces: []*committerpb.QueryNamespace{
				{NsId: ns, Keys: keys},
			},
		})
		if err == nil {
			var result []map[string]interface{}
			for _, rns := range rows.GetNamespaces() {
				for _, row := range rns.GetRows() {
					result = append(result, map[string]interface{}{
						"key":     formatKeyHex(row.GetKey()),
						"value":   formatValueHex(row.GetValue()),
						"version": row.GetVersion(),
					})
				}
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"namespace": ns, "rows": result})
			return
		}
		log.Printf("gRPC GetRows failed: %v", err)
	}

	// Fallback to DB
	records, err := h.pg.GetNamespaceState(r.Context(), ns, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"namespace": ns, "rows": records})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func splitKeys(s string) []string {
	var result []string
	current := ""
	for _, c := range s {
		if c == ',' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func formatKeyHex(key []byte) string {
	if len(key) == 0 {
		return ""
	}
	return string(key)
}

func formatValueHex(val []byte) string {
	if len(val) == 0 {
		return ""
	}
	if len(val) <= 256 {
		return string(val)
	}
	return string(val[:256]) + "..."
}
