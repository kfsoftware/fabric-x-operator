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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/hyperledger/fabric-config/protolator"
	"github.com/hyperledger/fabric-x-common/internaltools/configtxgen"
	"github.com/hyperledger/fabric-x-common/internaltools/configtxgen/genesisconfig"

	"github.com/go-logr/logr"
	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Organization represents a Fabric organization configuration
type Organization struct {
	MSPDir   string
	MSPType  string
	Policies map[string]*Policy
}

// Policy represents a Fabric policy
type Policy struct {
	Type string
	Rule string
}

// GenesisService handles the creation of genesis blocks
type GenesisService struct {
	client    client.Client
	logger    logr.Logger
	ChannelID string
}

// NewGenesisService creates a new GenesisService
func NewGenesisService(client client.Client, logger logr.Logger, channelID string) *GenesisService {
	return &GenesisService{
		client:    client,
		logger:    logger,
		ChannelID: channelID,
	}
}

// GenesisRequest represents a request to create a genesis block
type GenesisRequest struct {
	// Genesis resource
	Genesis   *v1alpha1.Genesis
	ChannelID string
}

// GenesisResult represents the result of genesis block creation
type GenesisResult struct {
	// Genesis block bytes
	GenesisBlock []byte

	// Shared config in protobuf format
	SharedConfigProto []byte

	// Shared config in JSON format for debug purposes
	SharedConfigJSON []byte

	// Error if any
	Error error
}

// CreateGenesisBlock creates a genesis block based on the Genesis resource
func (s *GenesisService) CreateGenesisBlock(ctx context.Context, req *GenesisRequest) (*GenesisResult, error) {
	s.logger.Info("Creating genesis block", "namespace", req.Genesis.Namespace, "name", req.Genesis.Name, "channel", s.ChannelID)

	// Validate that we have at least some organizations
	if len(req.Genesis.Spec.OrdererOrganizations) == 0 &&
		len(req.Genesis.Spec.ApplicationOrgs) == 0 {
		return nil, errors.New("no organizations specified in genesis spec")
	}

	// Create temporary directory for MSP files
	tempDir, err := os.MkdirTemp("", "genesis-msp")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp directory")
	}
	defer os.RemoveAll(tempDir)

	// Process organizations
	organizations, err := s.processOrganizations(ctx, req.Genesis.Spec.OrdererOrganizations)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process organizations")
	}

	// Process application organizations
	applicationOrgs, err := s.processApplicationOrganizations(ctx, req.Genesis.Spec.ApplicationOrgs, tempDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process application organizations")
	}

	// Merge all organizations
	allOrgs := append(organizations, applicationOrgs...)

	// Create orderer organizations
	ordererOrgs, err := s.createOrdererOrganizations(ctx, req.Genesis.Spec.OrdererOrganizations, allOrgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create orderer organizations")
	}

	// Generate SharedConfig from genesis CRD
	sharedConfigService := NewSharedConfigService(s.logger, s.client)
	sharedConfigReq := &SharedConfigRequest{
		Genesis:   req.Genesis,
		ChannelID: s.ChannelID,
	}

	sharedConfig, err := sharedConfigService.GenerateSharedConfig(ctx, sharedConfigReq)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate shared config")
	}

	// Validate that SharedConfig is not nil
	if sharedConfig == nil {
		return nil, errors.New("generated shared config is nil")
	}

	// Marshal SharedConfig to protobuf bytes
	sharedConfigBytes, err := proto.Marshal(sharedConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal shared config")
	}

	// Store SharedConfig in temporary file
	sharedConfigPath := filepath.Join(tempDir, "shared-config.pb")
	if err := os.WriteFile(sharedConfigPath, sharedConfigBytes, 0644); err != nil {
		return nil, errors.Wrap(err, "failed to write shared config to temporary file")
	}

	// Create genesis block with SharedConfig as consensus metadata
	genesisBlock, err := s.createGenesisBlock(allOrgs, ordererOrgs, sharedConfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create genesis block")
	}

	// Convert shared config to JSON for debug purposes
	sharedConfigJSON, err := s.decodeProtoToJSON("protos.SharedConfig", sharedConfigBytes)
	if err != nil {
		s.logger.Info("Failed to decode shared config to JSON", "error", err)
		// Continue with empty JSON if conversion fails
		sharedConfigJSON = []byte("{}")
	}

	return &GenesisResult{
		GenesisBlock:      genesisBlock,
		SharedConfigProto: sharedConfigBytes,
		SharedConfigJSON:  sharedConfigJSON,
	}, nil
}

// processOrganizations processes organizations with certificates from secrets
func (s *GenesisService) processOrganizations(ctx context.Context, organizations []v1alpha1.OrdererOrganization) ([]*genesisconfig.Organization, error) {
	var orgConfigs []*genesisconfig.Organization

	for _, org := range organizations {
		s.logger.Info("Processing organization", "name", org.Name)

		// Fetch signing CA certificate
		signCACert, err := s.fetchAndValidateX509Certificate(ctx, org.SignCACertRef)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch signing CA certificate for %s", org.Name)
		}

		// Fetch TLS CA certificate
		tlsCACert, err := s.fetchAndValidateX509Certificate(ctx, org.TLSCACertRef)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch TLS CA certificate for %s", org.Name)
		}

		// Provision MSP directory
		tempMSPDir, err := s.provisionMSPDirectory(org.MSPID, signCACert, tlsCACert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision MSP directory for %s", org.MSPID)
		}

		// Create organization configuration
		orgConfig := &genesisconfig.Organization{
			Name:    org.MSPID,
			ID:      org.MSPID,
			MSPDir:  tempMSPDir,
			MSPType: "bccsp",
			Policies: map[string]*genesisconfig.Policy{
				"Readers": {
					Type: "ImplicitMeta",
					Rule: "ANY Readers",
				},
				"Writers": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
				"Admins": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Admins",
				},
			},
		}

		orgConfigs = append(orgConfigs, orgConfig)
	}

	return orgConfigs, nil
}

// fetchAndValidateX509Certificate fetches and validates an X509 certificate from a Kubernetes Secret
// The certificate data is expected to be base64 encoded.
func (s *GenesisService) fetchAndValidateX509Certificate(ctx context.Context, secretRef v1alpha1.SecretKeyNSSelector) ([]byte, error) {
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
	s.logger.Info("Certificate data", "data", string(certData))
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

// createOrganizationConfig creates a genesisconfig.Organization from certificates
func (s *GenesisService) createOrganizationConfig(mspID string, adminCert, ordererCert []byte) (*genesisconfig.Organization, error) {
	// Create organization configuration
	org := &genesisconfig.Organization{
		Name:    mspID,
		ID:      mspID,
		MSPType: "bccsp",
		Policies: map[string]*genesisconfig.Policy{
			"Readers": {
				Type: "ImplicitMeta",
				Rule: "ANY Readers",
			},
			"Writers": {
				Type: "ImplicitMeta",
				Rule: "ANY Writers",
			},
			"Admins": {
				Type: "ImplicitMeta",
				Rule: "MAJORITY Admins",
			},
		},
	}

	// Use adminCert as signing CA cert and ordererCert as TLS CA cert
	// If ordererCert is not provided, use adminCert for both
	signCACert := adminCert
	tlsCACert := ordererCert
	if len(tlsCACert) == 0 {
		tlsCACert = adminCert
	}

	// Provision MSP directory
	tempMSPDir, err := s.provisionMSPDirectory(mspID, signCACert, tlsCACert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision MSP directory for %s", mspID)
	}

	org.MSPDir = tempMSPDir

	return org, nil
}

// createOrdererOrganizations creates orderer organizations from orderer organizations
func (s *GenesisService) createOrdererOrganizations(ctx context.Context, ordererOrgs []v1alpha1.OrdererOrganization, allOrgs []*genesisconfig.Organization) ([]*genesisconfig.Organization, error) {
	var resultOrgs []*genesisconfig.Organization

	// Group orderer organizations by MSP ID
	mspToOrgs := make(map[string]v1alpha1.OrdererOrganization)
	for _, org := range ordererOrgs {
		if _, ok := mspToOrgs[org.MSPID]; ok {
			return nil, errors.Errorf("multiple orderer organizations found for MSP ID %s", org.MSPID)
		}
		mspToOrgs[org.MSPID] = org
	}

	// Create orderer organizations
	for mspID, genesisOrg := range mspToOrgs {
		// Find the corresponding organization by ID to get certificates
		var sourceOrg *genesisconfig.Organization
		for _, o := range allOrgs {
			if o.ID == mspID {
				sourceOrg = o
				break
			}
		}

		if sourceOrg == nil {
			return nil, errors.Errorf("organization %s not found", mspID)
		}

		// Create a new orderer organization with proper MSP structure
		ordererOrg := &genesisconfig.Organization{
			Name:    sourceOrg.Name,
			ID:      sourceOrg.ID,
			MSPType: sourceOrg.MSPType,
			Policies: map[string]*genesisconfig.Policy{
				"Readers": {
					Type: "ImplicitMeta",
					Rule: "ANY Readers",
				},
				"Writers": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
				"Admins": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Admins",
				},
				"BlockValidation": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
			},
		}
		signCACert, err := s.fetchAndValidateX509Certificate(ctx, genesisOrg.SignCACertRef)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch signing CA certificate for %s", genesisOrg.Name)
		}
		tlsCACert, err := s.fetchAndValidateX509Certificate(ctx, genesisOrg.TLSCACertRef)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch TLS CA certificate for %s", genesisOrg.Name)
		}
		// Provision MSP directory for the orderer organization
		tempMSPDir, err := s.provisionMSPDirectory(mspID, signCACert, tlsCACert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to provision MSP directory for orderer org %s", mspID)
		}

		ordererOrg.MSPDir = tempMSPDir
		resultOrgs = append(resultOrgs, ordererOrg)
	}

	return resultOrgs, nil
}

// processApplicationOrganizations processes application organizations
func (s *GenesisService) processApplicationOrganizations(ctx context.Context, appOrgs []v1alpha1.ApplicationOrganization, tempDir string) ([]*genesisconfig.Organization, error) {
	var organizations []*genesisconfig.Organization

	for _, org := range appOrgs {
		s.logger.Info("Processing application organization", "name", org.Name)

		var orgConfig *genesisconfig.Organization
		var err error

		orgConfig, err = s.processExternalApplicationOrg(ctx, org, tempDir)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to process application organization %s", org.Name)
		}

		organizations = append(organizations, orgConfig)
	}

	return organizations, nil
}

// processExternalApplicationOrg processes an external application organization
func (s *GenesisService) processExternalApplicationOrg(ctx context.Context, org v1alpha1.ApplicationOrganization, tempDir string) (*genesisconfig.Organization, error) {
	// Fetch certificates from secrets
	signCACert, err := s.fetchAndValidateX509Certificate(ctx, org.SignCACertRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch signing CA certificate for %s", org.Name)
	}

	tlsCACert, err := s.fetchAndValidateX509Certificate(ctx, org.TLSCACertRef)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch TLS CA certificate for %s", org.Name)
	}

	// Provision MSP directory with the certificates
	tempMSPDir, err := s.provisionMSPDirectory(org.MSPID, signCACert, tlsCACert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision MSP directory for %s", org.MSPID)
	}

	// Create organization configuration
	orgConfig := &genesisconfig.Organization{
		Name:    org.MSPID,
		ID:      org.MSPID,
		MSPDir:  tempMSPDir,
		MSPType: "bccsp",
		Policies: map[string]*genesisconfig.Policy{
			"Readers": {
				Type: "ImplicitMeta",
				Rule: "ANY Readers",
			},
			"Writers": {
				Type: "ImplicitMeta",
				Rule: "ANY Writers",
			},
			"Admins": {
				Type: "ImplicitMeta",
				Rule: "MAJORITY Admins",
			},
		},
	}

	return orgConfig, nil
}

// provisionMSPDirectory creates a temporal MSP directory with the specified structure
func (s *GenesisService) provisionMSPDirectory(mspID string, signCACert, tlsCACert []byte) (string, error) {
	// Create temporal directory for this MSP
	tempMSPDir, err := os.MkdirTemp("", fmt.Sprintf("msp-%s-", mspID))
	if err != nil {
		return "", errors.Wrapf(err, "failed to create temp MSP directory for %s", mspID)
	}

	// Create all required directories
	dirs := []string{
		filepath.Join(tempMSPDir, "admincerts"),
		filepath.Join(tempMSPDir, "cacerts"),
		filepath.Join(tempMSPDir, "intermediatecerts"),
		filepath.Join(tempMSPDir, "keystore"),
		filepath.Join(tempMSPDir, "signcerts"),
		filepath.Join(tempMSPDir, "tlscacerts"),
		filepath.Join(tempMSPDir, "tlsintermediatecerts"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			os.RemoveAll(tempMSPDir)
			return "", errors.Wrapf(err, "failed to create directory %s", dir)
		}
	}

	// Write signing CA certificate
	if len(signCACert) > 0 {
		signCACertPath := filepath.Join(tempMSPDir, "cacerts", "ca.crt")
		if err := os.WriteFile(signCACertPath, signCACert, 0644); err != nil {
			os.RemoveAll(tempMSPDir)
			return "", errors.Wrap(err, "failed to write signing CA certificate")
		}
	}

	// Write TLS CA certificate
	if len(tlsCACert) > 0 {
		tlsCACertPath := filepath.Join(tempMSPDir, "tlscacerts", "ca.crt")
		if err := os.WriteFile(tlsCACertPath, tlsCACert, 0644); err != nil {
			os.RemoveAll(tempMSPDir)
			return "", errors.Wrap(err, "failed to write TLS CA certificate")
		}
	}

	// Create config.yaml for NodeOUs
	configYaml := `NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/ca.crt
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/ca.crt
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/ca.crt
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/ca.crt
    OrganizationalUnitIdentifier: orderer
`
	configPath := filepath.Join(tempMSPDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		os.RemoveAll(tempMSPDir)
		return "", errors.Wrap(err, "failed to write config.yaml")
	}

	return tempMSPDir, nil
}

// createGenesisBlock creates the genesis block programmatically
func (s *GenesisService) createGenesisBlock(allOrgs []*genesisconfig.Organization, ordererOrgs []*genesisconfig.Organization, sharedConfigPath string) ([]byte, error) {
	// Validate that we have at least one orderer organization
	if len(ordererOrgs) == 0 {
		return nil, errors.New("at least one orderer organization is required")
	}

	// Verify that the shared config file exists and is readable
	if _, err := os.Stat(sharedConfigPath); os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "shared config file does not exist: %s", sharedConfigPath)
	}

	// Try to read the file to ensure it's readable
	sharedConfigBytes, err := os.ReadFile(sharedConfigPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read shared config file: %s", sharedConfigPath)
	}

	s.logger.Info("Shared config file exists and is readable", "size", len(sharedConfigBytes))

	// Create a genesis profile programmatically
	profile := &genesisconfig.Profile{
		Orderer: &genesisconfig.Orderer{
			OrdererType: "arma",
			Arma: &genesisconfig.ConsensusMetadata{
				Path: sharedConfigPath,
			},
			Addresses:    []string{},
			BatchTimeout: 2 * time.Second,
			BatchSize: genesisconfig.BatchSize{
				MaxMessageCount:   500,
				AbsoluteMaxBytes:  10 * 1024 * 1024,
				PreferredMaxBytes: 2 * 1024 * 1024,
			},
			Organizations: ordererOrgs,
			Policies: map[string]*genesisconfig.Policy{
				"Readers": {
					Type: "ImplicitMeta",
					Rule: "ANY Readers",
				},
				"Writers": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
				"Admins": {
					Type: "ImplicitMeta",
					Rule: "ANY Admins",
				},
				"BlockValidation": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
			},
			Capabilities: map[string]bool{
				"V2_0": true,
			},
		},
		Application: &genesisconfig.Application{
			Organizations: allOrgs,
			Capabilities: map[string]bool{
				"V2_0": true,
			},
			Policies: map[string]*genesisconfig.Policy{
				"Readers": {
					Type: "ImplicitMeta",
					Rule: "ANY Readers",
				},
				"Writers": {
					Type: "ImplicitMeta",
					Rule: "ANY Writers",
				},
				"Admins": {
					Type: "ImplicitMeta",
					Rule: "MAJORITY Admins",
				},
			},
		},
		Capabilities: map[string]bool{
			"V2_0": true,
		},
		Policies: map[string]*genesisconfig.Policy{
			"Readers": {
				Type: "ImplicitMeta",
				Rule: "ANY Readers",
			},
			"Writers": {
				Type: "ImplicitMeta",
				Rule: "ANY Writers",
			},
			"Admins": {
				Type: "ImplicitMeta",
				Rule: "MAJORITY Admins",
			},
		},
	}
	s.logger.Info("Orderer orgs", "orgs", profile)
	s.logger.Info("Creating genesis block", "orderer orgs", len(ordererOrgs), "application orgs", len(allOrgs))
	genesisBlock, err := configtxgen.GetOutputBlock(profile, s.ChannelID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create genesis block")
	}

	// Validate that genesisBlock is not nil
	if genesisBlock == nil {
		return nil, errors.New("genesis block is nil")
	}

	// Marshal to protobuf
	blockBytes, err := proto.Marshal(genesisBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal genesis block")
	}

	return blockBytes, nil
}

// decodeProtoToJSON decodes a protobuf message and converts it to JSON
func (s *GenesisService) decodeProtoToJSON(msgName string, protobufData []byte) ([]byte, error) {
	mt, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(msgName))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find message type %s", msgName)
	}

	msgType := reflect.TypeOf(mt.Zero().Interface())
	if msgType == nil {
		return nil, errors.Errorf("message type %s is nil", msgName)
	}

	msg := reflect.New(msgType.Elem()).Interface().(proto.Message)

	err = proto.Unmarshal(protobufData, msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal protobuf")
	}

	var buf bytes.Buffer
	err = protolator.DeepMarshalJSON(&buf, msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal to JSON")
	}

	return buf.Bytes(), nil
}

// GetConfigTemplate retrieves the config template from ConfigMap
func (s *GenesisService) GetConfigTemplate(ctx context.Context, genesis *v1alpha1.Genesis) ([]byte, error) {
	configMap := &corev1.ConfigMap{}
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: genesis.Namespace,
		Name:      genesis.Spec.ConfigTemplate.ConfigMapName,
	}, configMap)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get config template ConfigMap")
	}

	templateData, exists := configMap.Data[genesis.Spec.ConfigTemplate.Key]
	if !exists {
		return nil, errors.Errorf("config template key %s not found in ConfigMap", genesis.Spec.ConfigTemplate.Key)
	}

	return []byte(templateData), nil
}
