//go:build fabricx

package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"time"
	"unsafe"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger-labs/fabric-token-sdk/token/services/network/fabricx/tms"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/api/committerpb"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var armaLogger = logging.MustGetLogger()

// directRouterBroadcaster is a tms.EnvelopeBroadcaster that sends envelopes
// directly to the Fabric-X router (arma) via a raw AtomicBroadcast gRPC call,
// bypassing fsc's ordering.Service (which only knows BFT/etcdraft/solo and
// rejects consensus type "arma").
//
// The router endpoint is read from core.yaml fabric.<network>.orderers[0].
// After a successful broadcast, it waits for finality via the channel's
// Finality service, mirroring fnsBroadcaster.
type directRouterBroadcaster struct {
	fnsProvider *fabric.NetworkServiceProvider

	mu    sync.Mutex
	conns map[string]*grpc.ClientConn // network -> cached conn
}

func newDirectRouterBroadcaster(fnsp *fabric.NetworkServiceProvider) *directRouterBroadcaster {
	return &directRouterBroadcaster{
		fnsProvider: fnsp,
		conns:       map[string]*grpc.ClientConn{},
	}
}

func (b *directRouterBroadcaster) dial(network string) (*grpc.ClientConn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if cc, ok := b.conns[network]; ok {
		return cc, nil
	}

	// The fsc ConfigService wrapper doesn't expose Orderers(); read the address
	// from ARMA_ROUTER_ADDRESS env var (set in run.sh) or fall back to GetString.
	addr := os.Getenv("ARMA_ROUTER_ADDRESS")
	if addr == "" {
		fns, err := b.fnsProvider.FabricNetworkService(network)
		if err != nil {
			return nil, fmt.Errorf("fns for [%s] not found: %w", network, err)
		}
		addr = fns.ConfigService().GetString(fmt.Sprintf("fabric.%s.orderers.0.address", network))
	}
	if addr == "" {
		return nil, fmt.Errorf("no arma router address for network [%s] — set ARMA_ROUTER_ADDRESS", network)
	}

	armaLogger.Infof("arma: dialing router at %s", addr)
	//nolint:staticcheck // grpc.Dial ensures a blocking connect; NewClient's lazy connect races with Send.
	cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("dial router [%s]: %w", addr, err)
	}
	b.conns[network] = cc
	return cc, nil
}

// Broadcast sends env to the router and waits for finality of txID.
func (b *directRouterBroadcaster) Broadcast(network, channel string, txID driver.TxID, env *cb.Envelope) error {
	cc, err := b.dial(network)
	if err != nil {
		return err
	}

	fns, err := b.fnsProvider.FabricNetworkService(network)
	if err != nil {
		return fmt.Errorf("fns for [%s] not found: %w", network, err)
	}
	ch, err := fns.Channel(channel)
	if err != nil {
		return fmt.Errorf("channel [%s]: %w", channel, err)
	}

	// Read query service address from config for finality fallback.
	queryAddr := os.Getenv("QUERY_SERVICE_ADDRESS")
	if queryAddr == "" {
		queryAddr = fns.ConfigService().GetString(fmt.Sprintf("fabric.%s.queryService.endpoints.0.address", network))
	}

	// start finality wait in parallel
	finalCh := make(chan error, 1)
	go func() { finalCh <- waitFinality(ch.Finality(), txID, queryAddr) }()

	armaLogger.Infof("arma: sending tx [%s] to router", txID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := ab.NewAtomicBroadcastClient(cc).Broadcast(ctx)
	if err != nil {
		return fmt.Errorf("create broadcast stream: %w", err)
	}
	if err := stream.Send(env); err != nil {
		return fmt.Errorf("send envelope: %w", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("recv broadcast response: %w", err)
	}
	_ = stream.CloseSend()
	armaLogger.Infof("arma: broadcast response status=%v info=%s", resp.GetStatus(), resp.GetInfo())
	if resp.GetStatus() != cb.Status_SUCCESS {
		return fmt.Errorf("broadcast rejected by router: status=%v info=%s", resp.GetStatus(), resp.GetInfo())
	}
	armaLogger.Infof("arma: router accepted tx [%s], waiting for finality", txID)

	return <-finalCh
}

// waitFinality first tries the notification-based finality with a short timeout.
// If that doesn't resolve (stream not connected, block committed before listener
// registered, etc.), it falls back to polling the query service directly.
func waitFinality(f driver.Finality, txID string, queryAddr string) error {
	// Phase 1: try notification-based finality with a timeout.
	// IsFinal respects context cancellation (wrappers.go:113), so a timeout
	// context will unblock it even if the notification stream never delivers.
	const notifyTimeout = 15 * time.Second
	const maxNotifyAttempts = 3

	for i := 0; i < maxNotifyAttempts; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		err := f.IsFinal(ctx, txID)
		cancel()
		if err == nil {
			armaLogger.Infof("arma: finality confirmed via notification for [%s]", txID)
			return nil
		}
		if errors.Is(err, io.EOF) {
			armaLogger.Warnf("arma: finality EOF for [%s] (attempt %d), retrying", txID, i+1)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if errors.Is(err, context.DeadlineExceeded) {
			armaLogger.Warnf("arma: finality notification timed out for [%s] after %v — falling back to query service", txID, notifyTimeout)
			break
		}
		// Other error (e.g., Unknown status from timeout response)
		armaLogger.Warnf("arma: finality check returned error for [%s]: %v — falling back to query service", txID, err)
		break
	}

	// Phase 2: poll query service directly.
	armaLogger.Infof("arma: using query service fallback for tx [%s] at [%s]", txID, queryAddr)
	return pollQueryService(queryAddr, txID)
}

// pollQueryService connects to the committer query service and checks if txID
// has been committed. Returns nil if the tx has COMMITTED status.
func pollQueryService(addr, txID string) error {
	if addr == "" {
		return fmt.Errorf("no query service address configured")
	}

	const pollRetries = 60
	const pollInterval = 2 * time.Second

	//nolint:staticcheck
	cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("dial query service [%s]: %w", addr, err)
	}
	defer cc.Close()

	client := committerpb.NewQueryServiceClient(cc)

	for i := 0; i < pollRetries; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := client.GetTransactionStatus(ctx, &committerpb.TxStatusQuery{
			TxIds: []string{txID},
		})
		cancel()
		if err != nil {
			armaLogger.Warnf("arma: query service GetTransactionStatus error (attempt %d): %v", i+1, err)
			time.Sleep(pollInterval)
			continue
		}
		for _, s := range resp.GetStatuses() {
			if s.GetRef().GetTxId() == txID {
				armaLogger.Infof("arma: query service reports tx [%s] status=%v", txID, s.GetStatus())
				if s.GetStatus() == committerpb.Status_COMMITTED {
					return nil
				}
				if s.GetStatus() != committerpb.Status_STATUS_UNSPECIFIED {
					return fmt.Errorf("tx [%s] rejected per query service: %v", txID, s.GetStatus())
				}
			}
		}
		// Status not yet available — tx may not have been committed yet
		armaLogger.Debugf("arma: tx [%s] not yet committed, polling again (attempt %d)", txID, i+1)
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("tx [%s] not committed after polling query service", txID)
}

// fnsSigningIdentityProviderShim mirrors the unexported fnsSigningIdentityProvider
// in fabric-token-sdk/.../fabricx/tms/common.go so we can construct a submitter
// via the exported tms.NewSubmitter constructor.
type fnsSigningIdentityProviderShim struct {
	fnsProvider *fabric.NetworkServiceProvider
}

type signerWithPublicVersion interface {
	tms.Signer
	GetPublicVersion() msp.Identity
}

type signerWrapper struct {
	signerWithPublicVersion
}

func (w *signerWrapper) Serialize() ([]byte, error) {
	return w.GetPublicVersion().Serialize()
}

func (p *fnsSigningIdentityProviderShim) DefaultSigningIdentity(network, channel string) (tms.Signer, error) {
	fns, err := p.fnsProvider.FabricNetworkService(network)
	if err != nil {
		return nil, fmt.Errorf("fns for [%s] not found: %w", network, err)
	}
	return &signerWrapper{
		signerWithPublicVersion: fns.LocalMembership().DefaultSigningIdentity().(signerWithPublicVersion),
	}, nil
}

func (p *fnsSigningIdentityProviderShim) DefaultIdentity(network, channel string) (view.Identity, error) {
	fns, err := p.fnsProvider.FabricNetworkService(network)
	if err != nil {
		return nil, fmt.Errorf("fns for [%s] not found: %w", network, err)
	}
	return fns.LocalMembership().DefaultIdentity(), nil
}

// NewArmaSubmitter builds a tms.Submitter that uses the direct router broadcaster.
//
// SIDE EFFECT: it also reaches into the underlying fabric network's
// ordering.Service via reflection and registers an "arma" entry in its
// Broadcasters map. Without that, the channel-config monitor's first call to
// orderingService.Configure("arma", ...) fails with "no broadcaster found for
// consensus [arma]" — and the retried Update then fails with the seq-0 error,
// blocking channel bootstrap entirely.
//
// We pre-create the network for "default" so the injection happens before any
// channel (and its monitor) is created.
func NewArmaSubmitter(fnsp *fabric.NetworkServiceProvider) tms.Submitter {
	br := newDirectRouterBroadcaster(fnsp)
	// NOTE: arma broadcaster registration into ordering.Service.Broadcasters is
	// done separately via an explicit Invoke in SDK.Install(), before channels
	// are created.  We do NOT register here to avoid double-registration.
	sip := &fnsSigningIdentityProviderShim{fnsProvider: fnsp}
	return tms.NewSubmitter(sip, br)
}

// registerArmaBroadcaster forces creation of the FNS for `network` and reaches
// (via reflection) into its underlying *generic.Network → *ordering.Service
// → Broadcasters map to register an "arma" handler that delegates to br.
//
// This is fragile (touches unexported fields) but unavoidable: fsc's
// ordering.Service hard-codes BFT/etcdraft/solo and there is no public hook.
func registerArmaBroadcaster(fnsp *fabric.NetworkServiceProvider, network string, br *directRouterBroadcaster) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()
	fns, err := fnsp.FabricNetworkService(network)
	if err != nil {
		return fmt.Errorf("get fns [%s]: %w", network, err)
	}

	// fns is *fabric.NetworkService → struct → field "fns" of type
	// driver.FabricNetworkService (interface) which holds *generic.Network.
	rv := reflect.ValueOf(fns).Elem() // fabric.NetworkService struct
	innerFld := rv.FieldByName("fns")
	if !innerFld.IsValid() {
		return errors.New("fns.fns field not found")
	}
	// Bypass the unexported guard.
	innerFld = reflect.NewAt(innerFld.Type(), unsafe.Pointer(innerFld.UnsafeAddr())).Elem()
	armaLogger.Infof("arma: innerFld kind=%v type=%v", innerFld.Kind(), innerFld.Type())
	// innerFld is the interface; unwrap → *generic.Network ; deref → struct.
	concrete := innerFld.Elem()
	armaLogger.Infof("arma: concrete kind=%v type=%v", concrete.Kind(), concrete.Type())
	netStruct := concrete
	if netStruct.Kind() == reflect.Ptr {
		netStruct = netStruct.Elem()
	}
	armaLogger.Infof("arma: netStruct kind=%v type=%v", netStruct.Kind(), netStruct.Type())
	if !netStruct.IsValid() || netStruct.Kind() != reflect.Struct {
		return fmt.Errorf("expected generic.Network struct, got kind %v", netStruct.Kind())
	}
	orderingField := netStruct.FieldByName("Ordering")
	armaLogger.Infof("arma: orderingField valid=%v kind=%v", orderingField.IsValid(), orderingField.Kind())
	if !orderingField.IsValid() {
		return errors.New("Ordering field not found on generic.Network")
	}
	if !orderingField.CanAddr() {
		// netStruct came from unwrapping an interface, which makes the inner
		// fields non-addressable. Re-anchor: get the *generic.Network pointer
		// from concrete (which is addressable interface elem) and deref via
		// unsafe to obtain an addressable struct.
		ptrToNet := concrete // Ptr to generic.Network
		armaLogger.Infof("arma: ptrToNet kind=%v canAddr=%v", ptrToNet.Kind(), ptrToNet.CanAddr())
		// concrete itself isn't addressable, but the value it points to IS,
		// because the underlying memory is the real *generic.Network. Use
		// reflect.NewAt with the pointer's UnsafePointer().
		netPtr := unsafe.Pointer(ptrToNet.Pointer())
		netStruct = reflect.NewAt(ptrToNet.Type().Elem(), netPtr).Elem()
		orderingField = netStruct.FieldByName("Ordering")
		armaLogger.Infof("arma: re-anchored orderingField canAddr=%v kind=%v", orderingField.CanAddr(), orderingField.Kind())
	}
	if orderingField.IsNil() {
		return errors.New("Ordering is nil")
	}
	// orderingField is driver.Ordering interface holding *ordering.Service.
	// .Elem() unwraps interface → *ordering.Service (Ptr). .Elem() again → struct.
	orderingPtr := orderingField.Elem() // *ordering.Service
	if orderingPtr.Kind() != reflect.Ptr {
		return fmt.Errorf("expected *ordering.Service ptr, got %v", orderingPtr.Kind())
	}
	// Re-anchor via unsafe to get an addressable struct (Broadcasters needs to be set).
	orderingService := reflect.NewAt(orderingPtr.Type().Elem(), unsafe.Pointer(orderingPtr.Pointer())).Elem()
	armaLogger.Infof("arma: orderingService kind=%v canAddr=%v type=%v", orderingService.Kind(), orderingService.CanAddr(), orderingService.Type())
	bcMap := orderingService.FieldByName("Broadcasters")
	if !bcMap.IsValid() {
		return errors.New("Broadcasters field not found on ordering.Service")
	}
	bcMap = reflect.NewAt(bcMap.Type(), unsafe.Pointer(bcMap.UnsafeAddr())).Elem()

	// Build the BroadcastFnc closure that delegates to our directRouterBroadcaster.
	fn := func(ctx context.Context, env *cb.Envelope) error {
		// channel name and txID are not available here; the monitor's Configure
		// path doesn't actually invoke the function — it's just a registration
		// check. But the post-monitor ordering.Broadcast WILL call this with a
		// real envelope; route it to the router with no finality wait (the
		// caller handles finality separately).
		cc, err := br.dial(network)
		if err != nil {
			return err
		}
		bctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		stream, err := ab.NewAtomicBroadcastClient(cc).Broadcast(bctx)
		if err != nil {
			return fmt.Errorf("create broadcast stream: %w", err)
		}
		if err := stream.Send(env); err != nil {
			return fmt.Errorf("send envelope: %w", err)
		}
		resp, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("recv broadcast response: %w", err)
		}
		_ = stream.CloseSend()
		if resp.GetStatus() != cb.Status_SUCCESS {
			return fmt.Errorf("router rejected: status=%v info=%s", resp.GetStatus(), resp.GetInfo())
		}
		return nil
	}

	bcMap.SetMapIndex(reflect.ValueOf("arma"), reflect.ValueOf(fn))
	armaLogger.Infof("arma: registered broadcaster on ordering.Service for network [%s]", network)
	return nil
}

