//go:build fabricx

package common

import (
	"sync"

	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-x-common/common/configtx"
	"github.com/hyperledger/fabric-x-common/protoutil"
)

// idempotentMembershipService wraps a MembershipService so that calling
// Update with the same config sequence number twice is a no-op instead of
// an error.  This breaks the retry-loop bug in the channel config monitor
// where a failed Configure() causes Update to be called again with the same
// envelope, which the underlying validator rejects (seq N != seq N+1).
type idempotentMembershipService struct {
	inner fdriver.MembershipService

	mu      sync.Mutex
	lastSeq int64 // -1 = never updated
}

func newIdempotentMembershipService(inner fdriver.MembershipService) *idempotentMembershipService {
	return &idempotentMembershipService{inner: inner, lastSeq: -1}
}

func (s *idempotentMembershipService) Update(env *cb.Envelope) error {
	seq, err := extractConfigSequence(env)
	if err != nil {
		// Can't parse → delegate and let the inner service decide.
		return s.inner.Update(env)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if seq == s.lastSeq {
		armaLogger.Debugf("idempotent membership: skipping duplicate Update for seq %d", seq)
		return nil
	}

	if err := s.inner.Update(env); err != nil {
		return err
	}
	s.lastSeq = seq
	return nil
}

func (s *idempotentMembershipService) OrdererConfig(cs fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	return s.inner.OrdererConfig(cs)
}

func (s *idempotentMembershipService) GetMSPIDs() []string {
	return s.inner.GetMSPIDs()
}

func (s *idempotentMembershipService) MSPManager() fdriver.MSPManager {
	return s.inner.MSPManager()
}

func (s *idempotentMembershipService) IsValid(identity view.Identity) error {
	return s.inner.IsValid(identity)
}

func (s *idempotentMembershipService) GetVerifier(identity view.Identity) (fdriver.Verifier, error) {
	return s.inner.GetVerifier(identity)
}

func (s *idempotentMembershipService) CheckACL(signedProp fdriver.SignedProposal) error {
	return s.inner.CheckACL(signedProp)
}

// extractConfigSequence parses an Envelope to get Config.Sequence.
func extractConfigSequence(env *cb.Envelope) (int64, error) {
	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return 0, err
	}
	cenv, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return 0, err
	}
	if cenv.Config == nil {
		return 0, nil
	}
	return int64(cenv.Config.Sequence), nil
}
