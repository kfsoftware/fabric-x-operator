//go:build fabricx

/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"context"
	"errors"
	"fmt"

	common "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fabricsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/state"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/config"
	committergrpc "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	fabricxsdk "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	dlog "github.com/hyperledger-labs/fabric-token-sdk/token/core/zkatdlog/nogh/v1/driver"
	"github.com/hyperledger-labs/fabric-token-sdk/token/sdk"
	tokensdk "github.com/hyperledger-labs/fabric-token-sdk/token/sdk/dig"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/network/fabricx"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/network/fabricx/pp"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/network/fabricx/tms"
	"go.uber.org/dig"
)

// SDK bypasses fabricxsdk.SDK so we can inject a custom ChannelProvider
// that wraps the membership service with idempotent Update() behavior.
// This prevents the seq-0 retry bug in the channel config monitor.
type SDK struct {
	common.SDK
}

// NewSDK wraps the plain fabric SDK (not fabricxsdk) so we can control
// all fabricx-specific Provides ourselves.
func NewSDK(registry services.Registry) *SDK {
	return &SDK{SDK: tokensdk.NewFrom(fabricsdk.NewSDK(registry))}
}

func (p *SDK) FabricEnabled() bool {
	return p.ConfigService().GetBool("fabric.enabled")
}

func (p *SDK) Install() error {
	if !p.FabricEnabled() {
		return p.SDK.Install()
	}

	err := errors.Join(
		// --- token SDK provides ---
		sdk.RegisterTokenDriverDependencies(p.Container()),
		p.Container().Provide(dlog.NewDriver, dig.Group("token-drivers")),

		// fabricx network driver for tokens
		p.Container().Provide(fabricx.NewDriver, dig.Group("network-drivers")),
		p.Container().Provide(NewArmaSubmitter, dig.As(new(tms.Submitter))),
		p.Container().Provide(tms.NewTMSDeployerService, dig.As(new(tms.DeployerService))),
		p.Container().Provide(pp.NewPublicParametersService),
		p.Container().Provide(digutils.Identity[*pp.PublicParametersService](), dig.As(new(pp.Loader))),

		// --- fabricx platform provides (replaces fabricxsdk.Install) ---
		p.Container().Provide(fabricxsdk.NewDriver, dig.Group("fabric-platform-drivers")),
		p.Container().Provide(NewArmaChannelProvider, dig.As(new(fabricxsdk.ChannelProvider))),
		p.Container().Provide(config.NewProvider, dig.As(
			new(committergrpc.ServiceConfigProvider),
			new(finality.ServiceConfigProvider),
			new(queryservice.ServiceConfigProvider),
		)),
		p.Container().Provide(committergrpc.NewClientProvider, dig.As(
			new(ledger.GRPCClientProvider),
			new(queryservice.GRPCClientProvider),
			new(finality.GRPCClientProvider),
		)),
		p.Container().Provide(ledger.NewProvider),
		p.Container().Provide(finality.NewListenerManagerProvider),
		p.Container().Provide(digutils.Identity[*finality.Provider](), dig.As(new(finality.ListenerManagerProvider))),
		p.Container().Provide(queryservice.NewProvider, dig.As(new(queryservice.Provider))),
	)
	if err != nil {
		return err
	}

	// Install the base fabric + token SDK (NOT fabricxsdk — we provided its deps above).
	if err := p.SDK.Install(); err != nil {
		return err
	}

	// Inject the arma broadcaster into ordering.Service.Broadcasters via reflection.
	if err := p.Container().Invoke(func(fnsp *fabric.NetworkServiceProvider) error {
		return registerArmaBroadcaster(fnsp, "default", newDirectRouterBroadcaster(fnsp))
	}); err != nil {
		return fmt.Errorf("inject arma broadcaster: %w", err)
	}

	return errors.Join(
		// fabricx backward-compat registrations (from fabricxsdk.Install)
		digutils.Register[finality.ListenerManagerProvider](p.Container()),
		digutils.Register[queryservice.Provider](p.Container()),
		digutils.Register[*ledger.Provider](p.Container()),
		// token SDK registrations
		digutils.Register[state.VaultService](p.Container()),
		digutils.Register[tms.DeployerService](p.Container()),
		digutils.Register[pp.Loader](p.Container()),
	)
}

// Start initializes fabricx providers (finality + ledger) before the parent
// Start creates channels and starts the config monitor.
func (p *SDK) Start(ctx context.Context) error {
	if p.FabricEnabled() {
		if err := p.Container().Invoke(func(in struct {
			dig.In
			FinalityProvider *finality.Provider
			LedgerProvider   *ledger.Provider
		}) error {
			in.FinalityProvider.Initialize(ctx)
			in.LedgerProvider.Initialize(ctx)
			return nil
		}); err != nil {
			return fmt.Errorf("initialize fabricx providers: %w", err)
		}
	}
	return p.SDK.Start(ctx)
}
