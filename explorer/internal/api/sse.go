package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"github.com/kfsoftware/fabric-x-operator/explorer/internal/decoder"
	"github.com/kfsoftware/fabric-x-operator/explorer/internal/grpcclient"
	"github.com/kfsoftware/fabric-x-operator/explorer/internal/pgstore"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SSEEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

func SSEHandler(client *grpcclient.Client, pg *pgstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ctx := r.Context()

		info, err := client.Blocks.GetBlockchainInfo(ctx, &emptypb.Empty{})
		if err != nil {
			writeSSE(w, flusher, SSEEvent{Type: "error", Data: err.Error()})
			return
		}
		lastHeight := info.GetHeight()

		writeSSE(w, flusher, SSEEvent{Type: "connected", Data: map[string]uint64{"height": lastHeight}})

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInfo, err := client.Blocks.GetBlockchainInfo(context.Background(), &emptypb.Empty{})
				if err != nil {
					continue
				}
				newHeight := newInfo.GetHeight()
				if newHeight > lastHeight {
					for num := lastHeight; num < newHeight; num++ {
						block, err := client.Blocks.GetBlockByNumber(context.Background(),
							&committerpb.BlockNumber{Number: num})
						if err != nil {
							continue
						}
						decoded, err := decoder.DecodeBlock(block)
						if err != nil {
							continue
						}
						writeSSE(w, flusher, SSEEvent{Type: "block", Data: decoded})
					}
					lastHeight = newHeight
				}
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event SSEEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("sse marshal error: %v", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
