//go:build fabricx

package common

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/identity"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/ledger"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

// NewArmaChannelProvider is a drop-in replacement for fabricxsdk.NewChannelProvider
// that wraps the membership service with idempotent Update() behavior.
// This prevents the seq-0 retry bug in the channel config monitor.
func NewArmaChannelProvider(in struct {
	dig.In
	ConfigProvider          config.Provider
	KVS                     *kvs.KVS
	LedgerProvider          *ledger.Provider
	Publisher               events.Publisher
	TracerProvider          trace.TracerProvider
	MetricsProvider         metrics.Provider
	QueryServiceProvider    queryservice.Provider
	ListenerManagerProvider finality.ListenerManagerProvider
	IdentityLoaders         []identity.NamedIdentityLoader `group:"identity-loaders"`
	EndpointService         identity.EndpointService
	IdProvider              identity.ViewIdentityProvider
	EnvelopeStore           fdriver.EnvelopeStore
	MetadataStore           fdriver.MetadataStore
	EndorseTxStore          fdriver.EndorseTxStore
	Drivers                 multiplexed.Driver
},
) generic.ChannelProvider {
	channelConfigProvider := generic.NewChannelConfigProvider(in.ConfigProvider)
	return channel.NewProvider(
		in.ConfigProvider,
		in.EnvelopeStore,
		in.MetadataStore,
		in.EndorseTxStore,
		in.Drivers,
		func(channelName string, configService fdriver.ConfigService, _ driver.VaultStore) (fdriver.Vault, error) {
			return vault.New(configService, channelName, in.QueryServiceProvider)
		},
		channelConfigProvider,
		func(channelName string, nw fdriver.FabricNetworkService, chaincodeManager fdriver.ChaincodeManager) (fdriver.Ledger, error) {
			return in.LedgerProvider.NewLedger(nw.Name(), channelName)
		},
		func(ch string, nw fdriver.FabricNetworkService, envelopeService fdriver.EnvelopeService, transactionService fdriver.EndorserTransactionService, v fdriver.RWSetInspector) (fdriver.RWSetLoader, error) {
			return rwset.NewLoader(nw.Name(), ch, envelopeService, transactionService, nw.TransactionManager(), v), nil
		},
		// delivery service constructor
		func(
			nw fdriver.FabricNetworkService,
			ch string,
			peerManager delivery.Services,
			l fdriver.Ledger,
			v delivery.Vault,
			callback fdriver.BlockCallback,
		) (generic.DeliveryService, error) {
			channelConfig, err := channelConfigProvider.GetChannelConfig(nw.Name(), ch)
			if err != nil {
				return nil, err
			}
			return delivery.NewService(
				ch,
				channelConfig,
				nw.Name(),
				nw.LocalMembership(),
				nw.ConfigService(),
				peerManager,
				l,
				v,
				nw.TransactionManager(),
				callback,
				in.TracerProvider,
				in.MetricsProvider,
				[]cb.HeaderType{cb.HeaderType_MESSAGE},
			)
		},
		// membership service — wrapped with idempotent Update
		func(channelName string) fdriver.MembershipService {
			return newIdempotentMembershipService(membership.NewService(channelName))
		},
		false,
		in.QueryServiceProvider,
		in.ListenerManagerProvider,
	)
}
