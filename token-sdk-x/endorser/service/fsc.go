package service

import (
	"context"
	"fmt"
	"os"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/config"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/network/fabricx/pp"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/network/fabricx/tms"
)

var logger = logging.MustGetLogger()

type FabricSmartClient struct {
	node *node.Node
}

func NewFSC(node *node.Node) *FabricSmartClient {
	return &FabricSmartClient{node: node}
}

// Init deploys token public parameters to the ledger.
// It reads PP from the local config file (publicParameters.path) because the
// ledger-based ppFetcher returns empty on first deployment.
func (f FabricSmartClient) Init(ctx context.Context) error {
	logger.Info("initializing token parameters")
	dep, err := tms.GetTMSDeployerService(f.node)
	if err != nil {
		return err
	}

	svc, err := f.node.GetService(reflect.TypeOf((*config.Service)(nil)))
	if err != nil {
		return fmt.Errorf("get config service: %w", err)
	}
	cfgSvc := svc.(*config.Service)

	confs, err := cfgSvc.Configurations()
	if err != nil {
		return fmt.Errorf("get TMS configurations: %w", err)
	}

	// Get PP fetcher to check if PP is already deployed on ledger.
	ppSvc, err := f.node.GetService(reflect.TypeOf((*pp.PublicParametersService)(nil)))
	if err != nil {
		return fmt.Errorf("get pp service: %w", err)
	}
	ppFetcher := ppSvc.(*pp.PublicParametersService)

	for _, conf := range confs {
		tmsID := conf.ID()
		ppPath := conf.GetString("publicParameters.path")
		if ppPath == "" {
			logger.Infof("no publicParameters.path for TMS [%s], skipping", tmsID)
			continue
		}

		// Check if PP is already deployed on ledger — skip re-deployment to
		// avoid bumping the MVCC version which desynchronizes the VersionKeeper.
		existing, fetchErr := ppFetcher.Fetch(tmsID.Network, tmsID.Channel, tmsID.Namespace)
		if fetchErr != nil {
			logger.Warnf("could not check existing PP for TMS [%s]: %v — will deploy", tmsID, fetchErr)
		} else if len(existing) > 0 {
			logger.Infof("PP already deployed on ledger for TMS [%s] (%d bytes), skipping", tmsID, len(existing))
			continue
		}

		logger.Infof("reading PP from [%s] for TMS [%s]", ppPath, tmsID)
		ppRaw, err := os.ReadFile(ppPath)
		if err != nil {
			return fmt.Errorf("read PP file [%s]: %w", ppPath, err)
		}
		if len(ppRaw) == 0 {
			return fmt.Errorf("PP file [%s] is empty", ppPath)
		}
		logger.Infof("deploying %d bytes of PP for TMS [%s]", len(ppRaw), tmsID)
		if err := dep.DeployTMSWithPP(tmsID, ppRaw); err != nil {
			return fmt.Errorf("deploy PP for [%s]: %w", tmsID, err)
		}
	}
	return nil
}
