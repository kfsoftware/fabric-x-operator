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
	"encoding/base64"
	"fmt"

	"github.com/hyperledger/fabric-x-orderer/config/protos"

	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// SharedConfigService handles the generation of SharedConfig from Genesis resources
type SharedConfigService struct {
	logger *logrus.Logger
}

// NewSharedConfigService creates a new SharedConfigService
func NewSharedConfigService(logger *logrus.Logger) *SharedConfigService {
	return &SharedConfigService{
		logger: logger,
	}
}

// SharedConfigRequest represents a request to generate SharedConfig
type SharedConfigRequest struct {
	Genesis   *v1alpha1.Genesis
	ChannelID string
}

// GenerateSharedConfig generates SharedConfig based on the Genesis resource
func (s *SharedConfigService) GenerateSharedConfig(ctx context.Context, req *SharedConfigRequest) (*protos.SharedConfig, error) {
	s.logger.Infof("Generating SharedConfig for %s/%s channel %s", req.Genesis.Namespace, req.Genesis.Name, req.ChannelID)

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

	sharedConfig := &protos.SharedConfig{
		PartiesConfig:   partiesConfig,
		ConsensusConfig: consensusConfig,
		BatchingConfig:  batchingConfig,
	}

	// Validate the final SharedConfig
	if sharedConfig == nil {
		return nil, errors.New("created shared config is nil")
	}

	s.logger.Infof("Successfully generated SharedConfig with %d parties", len(partiesConfig))
	return sharedConfig, nil
}

// generatePartiesConfig generates PartyConfig for each organization
func (s *SharedConfigService) generatePartiesConfig(ctx context.Context, genesis *v1alpha1.Genesis) ([]*protos.PartyConfig, error) {
	var partiesConfig []*protos.PartyConfig
	partyID := uint32(1) // Start with party ID 1

	s.logger.Infof("Processing %d internal organizations", len(genesis.Spec.InternalOrgs))
	// Process internal organizations
	for _, org := range genesis.Spec.InternalOrgs {
		partyConfig, err := s.createPartyConfigFromInternalOrg(ctx, org, partyID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create party config for internal org %s", org.Name)
		}
		partiesConfig = append(partiesConfig, partyConfig)
		partyID++
	}

	s.logger.Infof("Processing %d external organizations", len(genesis.Spec.ExternalOrgs))
	// Process external organizations
	for _, org := range genesis.Spec.ExternalOrgs {
		partyConfig, err := s.createPartyConfigFromExternalOrg(org, partyID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create party config for external org %s", org.Name)
		}
		partiesConfig = append(partiesConfig, partyConfig)
		partyID++
	}

	s.logger.Infof("Processing %d application organizations", len(genesis.Spec.ApplicationOrgs))
	// Process application organizations
	for _, org := range genesis.Spec.ApplicationOrgs {
		partyConfig, err := s.createPartyConfigFromApplicationOrg(ctx, org, partyID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create party config for application org %s", org.Name)
		}
		partiesConfig = append(partiesConfig, partyConfig)
		partyID++
	}

	s.logger.Infof("Generated %d party configurations", len(partiesConfig))
	// Return empty slice instead of nil if no organizations
	if len(partiesConfig) == 0 {
		return make([]*protos.PartyConfig, 0), nil
	}
	return partiesConfig, nil
}

// createPartyConfigFromInternalOrg creates PartyConfig from internal organization
func (s *SharedConfigService) createPartyConfigFromInternalOrg(ctx context.Context, org v1alpha1.InternalOrganization, partyID uint32) (*protos.PartyConfig, error) {
	// TODO: Fetch actual CA certificates from the CA resource
	// For now, using placeholder certificates
	caCerts := [][]byte{[]byte("placeholder-ca-cert")}
	tlsCACerts := [][]byte{[]byte("placeholder-tls-ca-cert")}

	partyConfig := &protos.PartyConfig{
		PartyID:    partyID,
		CACerts:    caCerts,
		TLSCACerts: tlsCACerts,
		RouterConfig: &protos.RouterNodeConfig{
			Host:    fmt.Sprintf("router.%s.example.com", org.Name),
			Port:    7050,
			TlsCert: []byte("placeholder-router-tls-cert"),
		},
		BatchersConfig: []*protos.BatcherNodeConfig{
			{
				ShardID:  1,
				Host:     fmt.Sprintf("batcher1.%s.example.com", org.Name),
				Port:     7051,
				SignCert: []byte("placeholder-batcher-sign-cert"),
				TlsCert:  []byte("placeholder-batcher-tls-cert"),
			},
		},
		ConsenterConfig: &protos.ConsenterNodeConfig{
			Host:     fmt.Sprintf("consenter.%s.example.com", org.Name),
			Port:     7052,
			SignCert: []byte("placeholder-consenter-sign-cert"),
			TlsCert:  []byte("placeholder-consenter-tls-cert"),
		},
		AssemblerConfig: &protos.AssemblerNodeConfig{
			Host:    fmt.Sprintf("assembler.%s.example.com", org.Name),
			Port:    7053,
			TlsCert: []byte("placeholder-assembler-tls-cert"),
		},
	}

	return partyConfig, nil
}

// createPartyConfigFromExternalOrg creates PartyConfig from external organization
func (s *SharedConfigService) createPartyConfigFromExternalOrg(org v1alpha1.ExternalOrganization, partyID uint32) (*protos.PartyConfig, error) {
	// Decode CA certificates
	signCACert, err := base64.StdEncoding.DecodeString(org.SignCert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode signing CA certificate for %s", org.Name)
	}

	tlsCACert, err := base64.StdEncoding.DecodeString(org.TLSCert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode TLS CA certificate for %s", org.Name)
	}

	caCerts := [][]byte{signCACert}
	tlsCACerts := [][]byte{tlsCACert}

	partyConfig := &protos.PartyConfig{
		PartyID:    partyID,
		CACerts:    caCerts,
		TLSCACerts: tlsCACerts,
		RouterConfig: &protos.RouterNodeConfig{
			Host:    fmt.Sprintf("router.%s.example.com", org.Name),
			Port:    7050,
			TlsCert: []byte("placeholder-router-tls-cert"),
		},
		BatchersConfig: []*protos.BatcherNodeConfig{
			{
				ShardID:  1,
				Host:     fmt.Sprintf("batcher1.%s.example.com", org.Name),
				Port:     7051,
				SignCert: []byte("placeholder-batcher-sign-cert"),
				TlsCert:  []byte("placeholder-batcher-tls-cert"),
			},
		},
		ConsenterConfig: &protos.ConsenterNodeConfig{
			Host:     fmt.Sprintf("consenter.%s.example.com", org.Name),
			Port:     7052,
			SignCert: []byte("placeholder-consenter-sign-cert"),
			TlsCert:  []byte("placeholder-consenter-tls-cert"),
		},
		AssemblerConfig: &protos.AssemblerNodeConfig{
			Host:    fmt.Sprintf("assembler.%s.example.com", org.Name),
			Port:    7053,
			TlsCert: []byte("placeholder-assembler-tls-cert"),
		},
	}

	return partyConfig, nil
}

// createPartyConfigFromApplicationOrg creates PartyConfig from application organization
func (s *SharedConfigService) createPartyConfigFromApplicationOrg(ctx context.Context, org v1alpha1.ApplicationOrganization, partyID uint32) (*protos.PartyConfig, error) {
	var caCerts [][]byte
	var tlsCACerts [][]byte

	switch org.Type {
	case "internal":
		if org.Internal == nil {
			return nil, errors.Errorf("internal configuration is required for internal organization %s", org.Name)
		}
		// TODO: Fetch actual CA certificates from the CA resource
		caCerts = [][]byte{[]byte("placeholder-ca-cert")}
		tlsCACerts = [][]byte{[]byte("placeholder-tls-ca-cert")}

	case "external":
		if org.External == nil {
			return nil, errors.Errorf("external configuration is required for external organization %s", org.Name)
		}
		// Decode CA certificates
		signCACert, err := base64.StdEncoding.DecodeString(org.External.SignCACert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode signing CA certificate for %s", org.Name)
		}

		tlsCACert, err := base64.StdEncoding.DecodeString(org.External.TLSCACert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode TLS CA certificate for %s", org.Name)
		}

		caCerts = [][]byte{signCACert}
		tlsCACerts = [][]byte{tlsCACert}

	default:
		return nil, errors.Errorf("invalid organization type %s for organization %s", org.Type, org.Name)
	}

	partyConfig := &protos.PartyConfig{
		PartyID:    partyID,
		CACerts:    caCerts,
		TLSCACerts: tlsCACerts,
		RouterConfig: &protos.RouterNodeConfig{
			Host:    fmt.Sprintf("router.%s.example.com", org.Name),
			Port:    7050,
			TlsCert: []byte("placeholder-router-tls-cert"),
		},
		BatchersConfig: []*protos.BatcherNodeConfig{
			{
				ShardID:  1,
				Host:     fmt.Sprintf("batcher1.%s.example.com", org.Name),
				Port:     7051,
				SignCert: []byte("placeholder-batcher-sign-cert"),
				TlsCert:  []byte("placeholder-batcher-tls-cert"),
			},
		},
		ConsenterConfig: &protos.ConsenterNodeConfig{
			Host:     fmt.Sprintf("consenter.%s.example.com", org.Name),
			Port:     7052,
			SignCert: []byte("placeholder-consenter-sign-cert"),
			TlsCert:  []byte("placeholder-consenter-tls-cert"),
		},
		AssemblerConfig: &protos.AssemblerNodeConfig{
			Host:    fmt.Sprintf("assembler.%s.example.com", org.Name),
			Port:    7053,
			TlsCert: []byte("placeholder-assembler-tls-cert"),
		},
	}

	return partyConfig, nil
}

// generateConsensusConfig generates ConsensusConfig
func (s *SharedConfigService) generateConsensusConfig(genesis *v1alpha1.Genesis) (*protos.ConsensusConfig, error) {
	// TODO: Configure SmartBFT parameters based on genesis configuration
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
	// TODO: Configure batching parameters based on genesis configuration
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
