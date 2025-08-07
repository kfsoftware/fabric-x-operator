/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package genesis

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/hyperledger/fabric-x-orderer/config/protos"
	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SharedConfigService handles the generation of SharedConfig from Genesis resources
type SharedConfigService struct {
	logger logr.Logger
	client client.Client
}

// NewSharedConfigService creates a new SharedConfigService
func NewSharedConfigService(logger logr.Logger, client client.Client) *SharedConfigService {
	return &SharedConfigService{
		logger: logger,
		client: client,
	}
}

// SharedConfigRequest represents a request to generate SharedConfig
type SharedConfigRequest struct {
	Genesis   *v1alpha1.Genesis
	ChannelID string
}

// GenerateSharedConfig generates SharedConfig based on the Genesis resource
func (s *SharedConfigService) GenerateSharedConfig(ctx context.Context, req *SharedConfigRequest) (*protos.SharedConfig, error) {
	s.logger.Info("Generating SharedConfig", "namespace", req.Genesis.Namespace, "name", req.Genesis.Name, "channel", req.ChannelID)

	// Validate input
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Genesis == nil {
		return nil, errors.New("genesis is nil")
	}
	if req.ChannelID == "" {
		return nil, errors.New("channelID is required")
	}

	// Generate parties configuration
	partiesConfig, err := s.generatePartiesConfig(ctx, req.Genesis)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate parties configuration")
	}

	// Generate consensus configuration
	consensusConfig, err := s.generateConsensusConfig(req.Genesis)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate consensus configuration")
	}

	// Generate batching configuration
	batchingConfig, err := s.generateBatchingConfig(req.Genesis)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate batching configuration")
	}

	// Validate that all configurations are not nil
	if partiesConfig == nil {
		return nil, errors.New("parties configuration is nil")
	}
	if consensusConfig == nil {
		return nil, errors.New("consensus configuration is nil")
	}
	if batchingConfig == nil {
		return nil, errors.New("batching configuration is nil")
	}

	// Validate minimum party requirements
	if len(partiesConfig) < 1 {
		return nil, errors.New("at least 1 party is required for consensus")
	}

	// Validate individual party configurations
	for i, party := range partiesConfig {
		if party == nil {
			return nil, errors.Errorf("party %d is nil", i)
		}
		if err := s.validatePartyConfig(party); err != nil {
			return nil, errors.Wrapf(err, "party %d validation failed", i)
		}
	}

	// Validate consensus configuration
	if err := s.validateConsensusConfig(consensusConfig); err != nil {
		return nil, errors.Wrap(err, "consensus configuration validation failed")
	}

	// Validate batching configuration
	if err := s.validateBatchingConfig(batchingConfig); err != nil {
		return nil, errors.Wrap(err, "batching configuration validation failed")
	}

	sharedConfig := &protos.SharedConfig{
		PartiesConfig:   partiesConfig,
		ConsensusConfig: consensusConfig,
		BatchingConfig:  batchingConfig,
	}

	// Validate the final SharedConfig
	if sharedConfig == nil {
		return nil, errors.New("created shared config is nil")
	}

	s.logger.Info("Successfully generated SharedConfig", "parties", len(partiesConfig))
	return sharedConfig, nil
}

// generatePartiesConfig generates PartyConfig for each party
func (s *SharedConfigService) generatePartiesConfig(ctx context.Context, genesis *v1alpha1.Genesis) ([]*protos.PartyConfig, error) {
	var partiesConfig []*protos.PartyConfig

	s.logger.Info("Processing parties", "parties", len(genesis.Spec.Parties))

	// Process parties directly from the Parties configuration
	for _, party := range genesis.Spec.Parties {
		partyConfig, err := s.createPartyConfigFromParty(ctx, party)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create party config for party %d", party.PartyID)
		}
		partiesConfig = append(partiesConfig, partyConfig)
	}

	s.logger.Info("Generated party configurations", "parties", len(partiesConfig))
	// Return empty slice instead of nil if no parties
	if len(partiesConfig) == 0 {
		return make([]*protos.PartyConfig, 0), nil
	}
	return partiesConfig, nil
}

// createPartyConfigFromParty creates PartyConfig from party configuration
func (s *SharedConfigService) createPartyConfigFromParty(ctx context.Context, party v1alpha1.PartyConfig) (*protos.PartyConfig, error) {
	// Fetch CA certificates
	var caCerts [][]byte
	for _, caCertRef := range party.CACerts {
		caCert, err := s.fetchCertificateFromSecret(ctx, caCertRef)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch CA certificate for party %d", party.PartyID)
		}
		caCerts = append(caCerts, caCert)
	}

	// Fetch TLS CA certificates
	var tlsCACerts [][]byte
	for _, tlsCACertRef := range party.TLSCACerts {
		tlsCACert, err := s.fetchCertificateFromSecret(ctx, tlsCACertRef)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch TLS CA certificate for party %d", party.PartyID)
		}
		tlsCACerts = append(tlsCACerts, tlsCACert)
	}

	// Validate router configuration is provided
	if party.RouterConfig == nil {
		return nil, errors.Errorf("router configuration is required for party %d", party.PartyID)
	}

	// Fetch router TLS certificate
	routerTlsCert, err := s.fetchCertificateFromSecret(ctx, party.RouterConfig.TLSCert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch router TLS certificate for party %d", party.PartyID)
	}

	// Validate batchers configuration is provided
	if len(party.BatchersConfig) == 0 {
		return nil, errors.Errorf("at least one batcher configuration is required for party %d", party.PartyID)
	}

	// Process batchers
	var batchersConfig []*protos.BatcherNodeConfig
	for _, batcher := range party.BatchersConfig {
		// Fetch batcher signing certificate
		batcherSignCert, err := s.fetchCertificateFromSecret(ctx, batcher.SignCert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch batcher signing certificate for party %d", party.PartyID)
		}

		// Fetch batcher TLS certificate
		batcherTlsCert, err := s.fetchCertificateFromSecret(ctx, batcher.TLSCert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch batcher TLS certificate for party %d", party.PartyID)
		}

		batchersConfig = append(batchersConfig, &protos.BatcherNodeConfig{
			ShardID:  uint32(batcher.ShardID),
			Host:     batcher.Host,
			Port:     uint32(batcher.Port),
			SignCert: batcherSignCert,
			TlsCert:  batcherTlsCert,
		})
	}

	// Validate consenter configuration is provided
	if party.ConsenterConfig == nil {
		return nil, errors.Errorf("consenter configuration is required for party %d", party.PartyID)
	}

	// Fetch consenter signing certificate
	consenterSignCert, err := s.fetchCertificateFromSecret(ctx, party.ConsenterConfig.SignCert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch consenter signing certificate for party %d", party.PartyID)
	}

	// Fetch consenter TLS certificate
	consenterTlsCert, err := s.fetchCertificateFromSecret(ctx, party.ConsenterConfig.TLSCert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch consenter TLS certificate for party %d", party.PartyID)
	}

	// Validate assembler configuration is provided
	if party.AssemblerConfig == nil {
		return nil, errors.Errorf("assembler configuration is required for party %d", party.PartyID)
	}

	// Fetch assembler TLS certificate
	assemblerTlsCert, err := s.fetchCertificateFromSecret(ctx, party.AssemblerConfig.TLSCert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch assembler TLS certificate for party %d", party.PartyID)
	}

	partyConfig := &protos.PartyConfig{
		PartyID:    uint32(party.PartyID),
		CACerts:    caCerts,
		TLSCACerts: tlsCACerts,
		RouterConfig: &protos.RouterNodeConfig{
			Host:    party.RouterConfig.Host,
			Port:    uint32(party.RouterConfig.Port),
			TlsCert: routerTlsCert,
		},
		BatchersConfig: batchersConfig,
		ConsenterConfig: &protos.ConsenterNodeConfig{
			Host:     party.ConsenterConfig.Host,
			Port:     uint32(party.ConsenterConfig.Port),
			SignCert: consenterSignCert,
			TlsCert:  consenterTlsCert,
		},
		AssemblerConfig: &protos.AssemblerNodeConfig{
			Host:    party.AssemblerConfig.Host,
			Port:    uint32(party.AssemblerConfig.Port),
			TlsCert: assemblerTlsCert,
		},
	}

	return partyConfig, nil
}

// generateConsensusConfig generates ConsensusConfig
func (s *SharedConfigService) generateConsensusConfig(genesis *v1alpha1.Genesis) (*protos.ConsensusConfig, error) {
	// Use the consensus configuration from Genesis spec if provided
	if genesis.Spec.ConsensusConfig != nil && genesis.Spec.ConsensusConfig.SmartBFT != nil {
		smartBFT := genesis.Spec.ConsensusConfig.SmartBFT
		smartBFTConfig := &protos.SmartBFTConfig{
			RequestBatchMaxCount:          uint64(smartBFT.RequestBatchMaxCount),
			RequestBatchMaxBytes:          uint64(smartBFT.RequestBatchMaxBytes),
			RequestBatchMaxInterval:       smartBFT.RequestBatchMaxInterval,
			IncomingMessageBufferSize:     uint64(smartBFT.IncomingMessageBufferSize),
			RequestPoolSize:               uint64(smartBFT.RequestPoolSize),
			RequestForwardTimeout:         smartBFT.RequestForwardTimeout,
			RequestComplainTimeout:        smartBFT.RequestComplainTimeout,
			RequestAutoRemoveTimeout:      smartBFT.RequestAutoRemoveTimeout,
			ViewChangeResendInterval:      smartBFT.ViewChangeResendInterval,
			ViewChangeTimeout:             smartBFT.ViewChangeTimeout,
			LeaderHeartbeatTimeout:        smartBFT.LeaderHeartbeatTimeout,
			LeaderHeartbeatCount:          uint64(smartBFT.LeaderHeartbeatCount),
			NumOfTicksBehindBeforeSyncing: uint64(smartBFT.NumOfTicksBehindBeforeSyncing),
			CollectTimeout:                smartBFT.CollectTimeout,
			SyncOnStart:                   smartBFT.SyncOnStart,
			SpeedUpViewChange:             smartBFT.SpeedUpViewChange,
			LeaderRotation:                smartBFT.LeaderRotation,
			DecisionsPerLeader:            uint64(smartBFT.DecisionsPerLeader),
			RequestMaxBytes:               uint64(smartBFT.RequestMaxBytes),
			RequestPoolSubmitTimeout:      smartBFT.RequestPoolSubmitTimeout,
		}

		consensusConfig := &protos.ConsensusConfig{
			SmartBFTConfig: smartBFTConfig,
		}

		return consensusConfig, nil
	}

	// Fallback to default configuration
	smartBFTConfig := &protos.SmartBFTConfig{
		RequestBatchMaxCount:          500,
		RequestBatchMaxBytes:          10 * 1024 * 1024, // 10MB
		RequestBatchMaxInterval:       "2s",
		IncomingMessageBufferSize:     200,
		RequestPoolSize:               1000,
		RequestForwardTimeout:         "3s",
		RequestComplainTimeout:        "10s",
		RequestAutoRemoveTimeout:      "60s",
		ViewChangeResendInterval:      "5s",
		ViewChangeTimeout:             "20s",
		LeaderHeartbeatTimeout:        "10s",
		LeaderHeartbeatCount:          10,
		NumOfTicksBehindBeforeSyncing: 10,
		CollectTimeout:                "10s",
		SyncOnStart:                   true,
		SpeedUpViewChange:             false,
		LeaderRotation:                true,
		DecisionsPerLeader:            1000,
		RequestMaxBytes:               100 * 1024 * 1024, // 100MB
		RequestPoolSubmitTimeout:      "5s",
	}

	consensusConfig := &protos.ConsensusConfig{
		SmartBFTConfig: smartBFTConfig,
	}

	return consensusConfig, nil
}

// generateBatchingConfig generates BatchingConfig
func (s *SharedConfigService) generateBatchingConfig(genesis *v1alpha1.Genesis) (*protos.BatchingConfig, error) {
	// Use the batching configuration from Genesis spec if provided
	if genesis.Spec.BatchingConfig != nil {
		batchingConfig := &protos.BatchingConfig{
			RequestMaxBytes: uint64(genesis.Spec.BatchingConfig.RequestMaxBytes),
		}

		if genesis.Spec.BatchingConfig.BatchTimeouts != nil {
			batchTimeouts := &protos.BatchTimeouts{
				BatchCreationTimeout:  genesis.Spec.BatchingConfig.BatchTimeouts.BatchCreationTimeout,
				FirstStrikeThreshold:  genesis.Spec.BatchingConfig.BatchTimeouts.FirstStrikeThreshold,
				SecondStrikeThreshold: genesis.Spec.BatchingConfig.BatchTimeouts.SecondStrikeThreshold,
				AutoRemoveTimeout:     genesis.Spec.BatchingConfig.BatchTimeouts.AutoRemoveTimeout,
			}
			batchingConfig.BatchTimeouts = batchTimeouts
		}

		if genesis.Spec.BatchingConfig.BatchSize != nil {
			batchSize := &protos.BatchSize{
				MaxMessageCount:   uint32(genesis.Spec.BatchingConfig.BatchSize.MaxMessageCount),
				AbsoluteMaxBytes:  uint32(genesis.Spec.BatchingConfig.BatchSize.AbsoluteMaxBytes),
				PreferredMaxBytes: uint32(genesis.Spec.BatchingConfig.BatchSize.PreferredMaxBytes),
			}
			batchingConfig.BatchSize = batchSize
		}

		return batchingConfig, nil
	}

	// Fallback to default configuration
	batchTimeouts := &protos.BatchTimeouts{
		BatchCreationTimeout:  "2s",
		FirstStrikeThreshold:  "5s",
		SecondStrikeThreshold: "10s",
		AutoRemoveTimeout:     "60s",
	}

	batchSize := &protos.BatchSize{
		MaxMessageCount:   500,
		AbsoluteMaxBytes:  10 * 1024 * 1024, // 10MB
		PreferredMaxBytes: 2 * 1024 * 1024,  // 2MB
	}

	batchingConfig := &protos.BatchingConfig{
		BatchTimeouts:   batchTimeouts,
		BatchSize:       batchSize,
		RequestMaxBytes: 100 * 1024 * 1024, // 100MB
	}

	return batchingConfig, nil
}

// validatePartyConfig validates a single party configuration
func (s *SharedConfigService) validatePartyConfig(party *protos.PartyConfig) error {
	if party == nil {
		return errors.New("party config is nil")
	}

	if party.PartyID == 0 {
		return errors.New("party ID must be greater than 0")
	}

	if len(party.CACerts) == 0 {
		return errors.New("at least one CA certificate is required")
	}

	if len(party.TLSCACerts) == 0 {
		return errors.New("at least one TLS CA certificate is required")
	}

	if party.RouterConfig == nil {
		return errors.New("router configuration is required")
	}

	if party.RouterConfig.Host == "" {
		return errors.New("router host is required")
	}

	if party.RouterConfig.Port == 0 {
		return errors.New("router port is required")
	}

	if len(party.RouterConfig.TlsCert) == 0 {
		return errors.New("router TLS certificate is required")
	}

	if len(party.BatchersConfig) == 0 {
		return errors.New("at least one batcher configuration is required")
	}

	for i, batcher := range party.BatchersConfig {
		if batcher == nil {
			return errors.Errorf("batcher %d is nil", i)
		}
		if batcher.Host == "" {
			return errors.Errorf("batcher %d host is required", i)
		}
		if batcher.Port == 0 {
			return errors.Errorf("batcher %d port is required", i)
		}
		if len(batcher.SignCert) == 0 {
			return errors.Errorf("batcher %d sign certificate is required", i)
		}
		if len(batcher.TlsCert) == 0 {
			return errors.Errorf("batcher %d TLS certificate is required", i)
		}
	}

	if party.ConsenterConfig == nil {
		return errors.New("consenter configuration is required")
	}

	if party.ConsenterConfig.Host == "" {
		return errors.New("consenter host is required")
	}

	if party.ConsenterConfig.Port == 0 {
		return errors.New("consenter port is required")
	}

	if len(party.ConsenterConfig.SignCert) == 0 {
		return errors.New("consenter sign certificate is required")
	}

	if len(party.ConsenterConfig.TlsCert) == 0 {
		return errors.New("consenter TLS certificate is required")
	}

	if party.AssemblerConfig == nil {
		return errors.New("assembler configuration is required")
	}

	if party.AssemblerConfig.Host == "" {
		return errors.New("assembler host is required")
	}

	if party.AssemblerConfig.Port == 0 {
		return errors.New("assembler port is required")
	}

	if len(party.AssemblerConfig.TlsCert) == 0 {
		return errors.New("assembler TLS certificate is required")
	}

	return nil
}

// validateConsensusConfig validates the consensus configuration
func (s *SharedConfigService) validateConsensusConfig(config *protos.ConsensusConfig) error {
	if config == nil {
		return errors.New("consensus config is nil")
	}

	if config.SmartBFTConfig == nil {
		return errors.New("SmartBFT configuration is required")
	}

	smartBFT := config.SmartBFTConfig
	if smartBFT.RequestBatchMaxCount == 0 {
		return errors.New("RequestBatchMaxCount must be greater than 0")
	}

	if smartBFT.RequestBatchMaxBytes == 0 {
		return errors.New("RequestBatchMaxBytes must be greater than 0")
	}

	if smartBFT.RequestBatchMaxInterval == "" {
		return errors.New("RequestBatchMaxInterval is required")
	}

	return nil
}

// validateBatchingConfig validates the batching configuration
func (s *SharedConfigService) validateBatchingConfig(config *protos.BatchingConfig) error {
	if config == nil {
		return errors.New("batching config is nil")
	}

	if config.BatchTimeouts == nil {
		return errors.New("batch timeouts configuration is required")
	}

	if config.BatchSize == nil {
		return errors.New("batch size configuration is required")
	}

	batchSize := config.BatchSize
	if batchSize.MaxMessageCount == 0 {
		return errors.New("MaxMessageCount must be greater than 0")
	}

	if batchSize.AbsoluteMaxBytes == 0 {
		return errors.New("AbsoluteMaxBytes must be greater than 0")
	}

	if batchSize.PreferredMaxBytes == 0 {
		return errors.New("PreferredMaxBytes must be greater than 0")
	}

	return nil
}

// fetchCertificateFromSecret fetches and validates an X509 certificate from a Kubernetes Secret
// The certificate data is expected to be base64 encoded.
func (s *SharedConfigService) fetchCertificateFromSecret(ctx context.Context, secretRef v1alpha1.SecretKeyNSSelector) ([]byte, error) {
	secret := &corev1.Secret{}
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: secretRef.Namespace,
		Name:      secretRef.Name,
	}, secret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get secret %s/%s", secretRef.Namespace, secretRef.Name)
	}

	certData, exists := secret.Data[secretRef.Key]
	if !exists {
		return nil, errors.Errorf("key %s not found in secret %s/%s", secretRef.Key, secretRef.Namespace, secretRef.Name)
	}

	// Validate that the data is not empty
	if len(certData) == 0 {
		return nil, errors.Errorf("certificate data is empty for key %s in secret %s/%s", secretRef.Key, secretRef.Namespace, secretRef.Name)
	}

	// Validate that the data is a valid X509 certificate
	_, err = utils.ParseX509Certificate([]byte(certData))
	if err != nil {
		return nil, errors.Wrapf(err, "invalid X509 certificate for key %s in secret %s/%s", secretRef.Key, secretRef.Namespace, secretRef.Name)
	}

	return certData, nil
}
