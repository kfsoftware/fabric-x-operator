package grpcclient

import (
	"context"
	"fmt"

	"github.com/hyperledger/fabric-x-common/api/committerpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the committer gRPC service clients.
// BlockQueryService lives on the sidecar; QueryService+Notifier on the query-service.
type Client struct {
	conns    []*grpc.ClientConn
	Blocks   committerpb.BlockQueryServiceClient
	Query    committerpb.QueryServiceClient
	Notifier committerpb.NotifierClient
}

func Dial(ctx context.Context, queryAddr, sidecarAddr string) (*Client, error) {
	queryConn, err := grpc.NewClient(queryAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial query %s: %w", queryAddr, err)
	}

	sidecarConn, err := grpc.NewClient(sidecarAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		queryConn.Close()
		return nil, fmt.Errorf("grpc dial sidecar %s: %w", sidecarAddr, err)
	}

	return &Client{
		conns:    []*grpc.ClientConn{queryConn, sidecarConn},
		Blocks:   committerpb.NewBlockQueryServiceClient(sidecarConn),
		Query:    committerpb.NewQueryServiceClient(queryConn),
		Notifier: committerpb.NewNotifierClient(queryConn),
	}, nil
}

func (c *Client) Close() error {
	for _, conn := range c.conns {
		conn.Close()
	}
	return nil
}
