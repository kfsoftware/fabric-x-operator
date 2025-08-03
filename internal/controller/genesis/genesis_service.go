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
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/hyperledger/fabric-x-common/internaltools/configtxgen"
	"github.com/hyperledger/fabric-x-common/internaltools/configtxgen/genesisconfig"

	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	logger    *logrus.Logger
	ChannelID string
}

// NewGenesisService creates a new GenesisService
func NewGenesisService(client client.Client, logger *logrus.Logger, channelID string) *GenesisService {
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

	// Error if any
	Error error
}

// CreateGenesisBlock creates a genesis block based on the Genesis resource
func (s *GenesisService) CreateGenesisBlock(ctx context.Context, req *GenesisRequest) ([]byte, error) {
	s.logger.Infof("Creating genesis block for %s/%s channel %s", req.Genesis.Namespace, req.Genesis.Name, s.ChannelID)

	// Validate that we have at least some organizations
	if len(req.Genesis.Spec.InternalOrgs) == 0 &&
		len(req.Genesis.Spec.ExternalOrgs) == 0 &&
		len(req.Genesis.Spec.ApplicationOrgs) == 0 {
		return nil, errors.New("no organizations specified in genesis spec")
	}

	// Create temporary directory for MSP files
	tempDir, err := os.MkdirTemp("", "genesis-msp")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temp directory")
	}
	defer os.RemoveAll(tempDir)

	// Fetch certificates for internal organizations
	internalOrgs, err := s.fetchInternalOrganizationCertificates(ctx, req.Genesis.Spec.InternalOrgs, tempDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch internal organization certificates")
	}

	// Process external organizations
	externalOrgs, err := s.processExternalOrganizations(req.Genesis.Spec.ExternalOrgs, tempDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process external organizations")
	}

	// Process application organizations
	applicationOrgs, err := s.processApplicationOrganizations(ctx, req.Genesis.Spec.ApplicationOrgs, tempDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process application organizations")
	}

	// Merge all organizations
	allOrgs := append(internalOrgs, externalOrgs...)
	allOrgs = append(allOrgs, applicationOrgs...)

	// Create orderer organizations
	ordererOrgs, err := s.createOrdererOrganizations(req.Genesis.Spec.OrdererNodes, allOrgs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create orderer organizations")
	}

	// Generate SharedConfig from genesis CRD
	sharedConfigService := NewSharedConfigService(s.logger)
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

	return genesisBlock, nil
}

// fetchInternalOrganizationCertificates fetches certificates from Fabric CAs for internal organizations
func (s *GenesisService) fetchInternalOrganizationCertificates(ctx context.Context, internalOrgs []v1alpha1.InternalOrganization, tempDir string) ([]*genesisconfig.Organization, error) {
	var organizations []*genesisconfig.Organization

	for _, org := range internalOrgs {
		s.logger.Infof("Fetching certificates for internal organization %s", org.Name)

		// Get CA resource
		ca := &v1alpha1.CA{}
		err := s.client.Get(ctx, client.ObjectKey{
			Namespace: org.CAReference.Namespace,
			Name:      org.CAReference.Name,
		}, ca)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get CA resource %s/%s", org.CAReference.Namespace, org.CAReference.Name)
		}

		// Fetch admin certificate
		adminCert, err := s.fetchCertificateFromCA(ctx, ca, org.AdminIdentity)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch admin certificate for %s", org.Name)
		}

		// Fetch orderer certificate if specified
		var ordererCert []byte
		if org.OrdererIdentity != "" {
			ordererCert, err = s.fetchCertificateFromCA(ctx, ca, org.OrdererIdentity)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to fetch orderer certificate for %s", org.Name)
			}
		}

		// Create organization configuration
		orgConfig, err := s.createOrganizationConfig(org.MSPID, adminCert, ordererCert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create organization config for %s", org.Name)
		}

		organizations = append(organizations, orgConfig)
	}

	return organizations, nil
}

// fetchCertificateFromCA fetches a certificate from a Fabric CA
func (s *GenesisService) fetchCertificateFromCA(ctx context.Context, ca *v1alpha1.CA, identityName string) ([]byte, error) {
	// For testing purposes, we'll use the CA certificate from the CA status
	// In a real implementation, you would:
	// 1. Get the CA's TLS certificate from its secret
	// 2. Use the Fabric CA client to enroll the identity
	// 3. Return the certificate

	if ca.Status.CACert == "" {
		return nil, errors.New("CA certificate not available in CA status")
	}

	// For now, return the CA certificate as a placeholder for the identity
	// TODO: Implement actual CA certificate fetching using Fabric CA client
	return []byte(ca.Status.CACert), nil
}

// processExternalOrganizations processes external organizations with provided certificates
func (s *GenesisService) processExternalOrganizations(externalOrgs []v1alpha1.ExternalOrganization, tempDir string) ([]*genesisconfig.Organization, error) {
	var organizations []*genesisconfig.Organization

	for _, org := range externalOrgs {
		s.logger.Infof("Processing external organization %s", org.Name)

		// Decode certificates
		adminCert, err := base64.StdEncoding.DecodeString(org.AdminCert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode admin certificate for %s", org.Name)
		}

		var ordererCert []byte
		if org.OrdererCert != "" {
			ordererCert, err = base64.StdEncoding.DecodeString(org.OrdererCert)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to decode orderer certificate for %s", org.Name)
			}
		}

		// Create organization configuration
		orgConfig, err := s.createOrganizationConfig(org.MSPID, adminCert, ordererCert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create organization config for %s", org.Name)
		}

		organizations = append(organizations, orgConfig)
	}

	return organizations, nil
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

	// For now, we'll use a temporary MSP directory approach
	// In a real implementation, you would create the MSP structure in memory
	// and use the fabric-x-common MSP utilities to load it
	tempMSPDir, err := os.MkdirTemp("", fmt.Sprintf("msp-%s-", mspID))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create temp MSP directory for %s", mspID)
	}

	// Create MSP structure
	if err := s.createMSPStructure(tempMSPDir, adminCert, ordererCert); err != nil {
		os.RemoveAll(tempMSPDir)
		return nil, errors.Wrapf(err, "failed to create MSP structure for %s", mspID)
	}

	org.MSPDir = tempMSPDir

	return org, nil
}

// createMSPStructure creates the MSP directory structure
func (s *GenesisService) createMSPStructure(mspDir string, adminCert, ordererCert []byte) error {
	// Create directories
	dirs := []string{
		filepath.Join(mspDir, "cacerts"),
		filepath.Join(mspDir, "tlscacerts"),
		filepath.Join(mspDir, "admincerts"),
		filepath.Join(mspDir, "signcerts"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory %s", dir)
		}
	}

	// Write CA certificate (using admin cert as placeholder)
	if len(adminCert) > 0 {
		caCertPath := filepath.Join(mspDir, "cacerts", "cacert.pem")
		if err := os.WriteFile(caCertPath, adminCert, 0644); err != nil {
			return errors.Wrap(err, "failed to write CA certificate")
		}
	}

	// Write admin certificate
	if len(adminCert) > 0 {
		adminCertPath := filepath.Join(mspDir, "admincerts", "admincert.pem")
		if err := os.WriteFile(adminCertPath, adminCert, 0644); err != nil {
			return errors.Wrap(err, "failed to write admin certificate")
		}
	}

	// Write orderer certificate if provided
	if len(ordererCert) > 0 {
		ordererCertPath := filepath.Join(mspDir, "signcerts", "orderercert.pem")
		if err := os.WriteFile(ordererCertPath, ordererCert, 0644); err != nil {
			return errors.Wrap(err, "failed to write orderer certificate")
		}
	}

	// Create config.yaml for NodeOUs
	configYaml := `NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/cacert.pem
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/cacert.pem
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/cacert.pem
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/cacert.pem
    OrganizationalUnitIdentifier: orderer
`
	configPath := filepath.Join(mspDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		return errors.Wrap(err, "failed to write config.yaml")
	}

	return nil
}

// createOrdererOrganizations creates orderer organizations from orderer nodes
func (s *GenesisService) createOrdererOrganizations(ordererNodes []v1alpha1.OrdererNode, allOrgs []*genesisconfig.Organization) ([]*genesisconfig.Organization, error) {
	var ordererOrgs []*genesisconfig.Organization

	// Group orderer nodes by MSP ID
	mspToNodes := make(map[string][]v1alpha1.OrdererNode)
	for _, node := range ordererNodes {
		mspToNodes[node.MSPID] = append(mspToNodes[node.MSPID], node)
	}

	// Create orderer organizations
	for mspID := range mspToNodes {
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

		// Create MSP directory for the orderer organization
		tempMSPDir, err := os.MkdirTemp("", fmt.Sprintf("orderer-msp-%s-", mspID))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create temp MSP directory for orderer org %s", mspID)
		}

		// Copy MSP structure from source organization
		if err := s.copyMSPStructure(sourceOrg.MSPDir, tempMSPDir); err != nil {
			os.RemoveAll(tempMSPDir)
			return nil, errors.Wrapf(err, "failed to copy MSP structure for orderer org %s", mspID)
		}

		ordererOrg.MSPDir = tempMSPDir
		ordererOrgs = append(ordererOrgs, ordererOrg)
	}

	return ordererOrgs, nil
}

// copyMSPStructure copies the MSP directory structure from source to destination
func (s *GenesisService) copyMSPStructure(sourceDir, destDir string) error {
	// Create destination directories
	dirs := []string{
		filepath.Join(destDir, "cacerts"),
		filepath.Join(destDir, "tlscacerts"),
		filepath.Join(destDir, "admincerts"),
		filepath.Join(destDir, "signcerts"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory %s", dir)
		}
	}

	// Copy files from source to destination
	sourceFiles := []string{
		filepath.Join(sourceDir, "admincerts", "admincert.pem"),
		filepath.Join(sourceDir, "signcerts", "orderercert.pem"),
		filepath.Join(sourceDir, "config.yaml"),
	}

	for _, sourceFile := range sourceFiles {
		if _, err := os.Stat(sourceFile); err == nil {
			// File exists, copy it
			destFile := filepath.Join(destDir, filepath.Base(sourceFile))
			if err := copyFile(sourceFile, destFile); err != nil {
				return errors.Wrapf(err, "failed to copy file %s to %s", sourceFile, destFile)
			}
		}
	}

	// Copy CA certificates
	sourceCACert := filepath.Join(sourceDir, "cacerts", "cacert.pem")
	if _, err := os.Stat(sourceCACert); err == nil {
		destCACert := filepath.Join(destDir, "cacerts", "cacert.pem")
		if err := copyFile(sourceCACert, destCACert); err != nil {
			return errors.Wrapf(err, "failed to copy CA certificate")
		}
	}

	return nil
}

// copyFile copies a file from source to destination
func copyFile(source, dest string) error {
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// processApplicationOrganizations processes application organizations
func (s *GenesisService) processApplicationOrganizations(ctx context.Context, appOrgs []v1alpha1.ApplicationOrganization, tempDir string) ([]*genesisconfig.Organization, error) {
	var organizations []*genesisconfig.Organization

	for _, org := range appOrgs {
		s.logger.Infof("Processing application organization %s", org.Name)

		var orgConfig *genesisconfig.Organization
		var err error

		switch org.Type {
		case "internal":
			if org.Internal == nil {
				return nil, errors.Errorf("internal configuration is required for internal organization %s", org.Name)
			}
			orgConfig, err = s.processInternalApplicationOrg(ctx, org, tempDir)
		case "external":
			if org.External == nil {
				return nil, errors.Errorf("external configuration is required for external organization %s", org.Name)
			}
			orgConfig, err = s.processExternalApplicationOrg(org, tempDir)
		default:
			return nil, errors.Errorf("invalid organization type %s for organization %s", org.Type, org.Name)
		}

		if err != nil {
			return nil, errors.Wrapf(err, "failed to process application organization %s", org.Name)
		}

		organizations = append(organizations, orgConfig)
	}

	return organizations, nil
}

// processInternalApplicationOrg processes an internal application organization
func (s *GenesisService) processInternalApplicationOrg(ctx context.Context, org v1alpha1.ApplicationOrganization, tempDir string) (*genesisconfig.Organization, error) {
	// Get CA resource
	ca := &v1alpha1.CA{}
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: org.Internal.CAReference.Namespace,
		Name:      org.Internal.CAReference.Name,
	}, ca)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get CA resource %s/%s", org.Internal.CAReference.Namespace, org.Internal.CAReference.Name)
	}

	// Fetch admin certificate
	adminCert, err := s.fetchCertificateFromCA(ctx, ca, org.Internal.AdminIdentity)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch admin certificate for %s", org.Name)
	}

	// Fetch peer certificate if specified
	var peerCert []byte
	if org.Internal.PeerIdentity != "" {
		peerCert, err = s.fetchCertificateFromCA(ctx, ca, org.Internal.PeerIdentity)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to fetch peer certificate for %s", org.Name)
		}
	}

	// Create organization configuration
	return s.createOrganizationConfig(org.MSPID, adminCert, peerCert)
}

// processExternalApplicationOrg processes an external application organization
func (s *GenesisService) processExternalApplicationOrg(org v1alpha1.ApplicationOrganization, tempDir string) (*genesisconfig.Organization, error) {
	// Decode certificates
	signCACert, err := base64.StdEncoding.DecodeString(org.External.SignCACert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode signing CA certificate for %s", org.Name)
	}

	tlsCACert, err := base64.StdEncoding.DecodeString(org.External.TLSCACert)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode TLS CA certificate for %s", org.Name)
	}

	var adminCert []byte
	if org.External.AdminCert != "" {
		adminCert, err = base64.StdEncoding.DecodeString(org.External.AdminCert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode admin certificate for %s", org.Name)
		}
	}

	var peerCert []byte
	if org.External.PeerCert != "" {
		peerCert, err = base64.StdEncoding.DecodeString(org.External.PeerCert)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode peer certificate for %s", org.Name)
		}
	}

	// Create MSP directory for this organization
	orgMSPDir := filepath.Join(tempDir, org.MSPID)
	if err := os.MkdirAll(orgMSPDir, 0755); err != nil {
		return nil, errors.Wrapf(err, "failed to create MSP directory for %s", org.MSPID)
	}

	// Create MSP structure with the decoded certificates
	if err := s.createMSPStructureWithCerts(orgMSPDir, signCACert, tlsCACert, adminCert, peerCert); err != nil {
		return nil, errors.Wrapf(err, "failed to create MSP structure for %s", org.MSPID)
	}

	// Create organization configuration
	orgConfig := &genesisconfig.Organization{
		Name:    org.MSPID,
		ID:      org.MSPID,
		MSPDir:  orgMSPDir,
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

// createMSPStructureWithCerts creates the MSP directory structure with provided certificates
func (s *GenesisService) createMSPStructureWithCerts(mspDir string, signCACert, tlsCACert, adminCert, peerCert []byte) error {
	// Create directories
	dirs := []string{
		filepath.Join(mspDir, "cacerts"),
		filepath.Join(mspDir, "tlscacerts"),
		filepath.Join(mspDir, "admincerts"),
		filepath.Join(mspDir, "signcerts"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory %s", dir)
		}
	}

	// Write signing CA certificate
	if len(signCACert) > 0 {
		signCACertPath := filepath.Join(mspDir, "cacerts", "cacert.pem")
		if err := os.WriteFile(signCACertPath, signCACert, 0644); err != nil {
			return errors.Wrap(err, "failed to write signing CA certificate")
		}
	}

	// Write TLS CA certificate
	if len(tlsCACert) > 0 {
		tlsCACertPath := filepath.Join(mspDir, "tlscacerts", "tlscacert.pem")
		if err := os.WriteFile(tlsCACertPath, tlsCACert, 0644); err != nil {
			return errors.Wrap(err, "failed to write TLS CA certificate")
		}
	}

	// Write admin certificate
	if len(adminCert) > 0 {
		adminCertPath := filepath.Join(mspDir, "admincerts", "admincert.pem")
		if err := os.WriteFile(adminCertPath, adminCert, 0644); err != nil {
			return errors.Wrap(err, "failed to write admin certificate")
		}
	}

	// Write peer certificate if provided
	if len(peerCert) > 0 {
		peerCertPath := filepath.Join(mspDir, "signcerts", "peercert.pem")
		if err := os.WriteFile(peerCertPath, peerCert, 0644); err != nil {
			return errors.Wrap(err, "failed to write peer certificate")
		}
	}

	// Create config.yaml for NodeOUs
	configYaml := `NodeOUs:
  Enable: true
  ClientOUIdentifier:
    Certificate: cacerts/cacert.pem
    OrganizationalUnitIdentifier: client
  PeerOUIdentifier:
    Certificate: cacerts/cacert.pem
    OrganizationalUnitIdentifier: peer
  AdminOUIdentifier:
    Certificate: cacerts/cacert.pem
    OrganizationalUnitIdentifier: admin
  OrdererOUIdentifier:
    Certificate: cacerts/cacert.pem
    OrganizationalUnitIdentifier: orderer
`
	configPath := filepath.Join(mspDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		return errors.Wrap(err, "failed to write config.yaml")
	}

	return nil
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

	s.logger.Infof("Shared config file exists and is readable, size: %d bytes", len(sharedConfigBytes))

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

	s.logger.Infof("Creating genesis block with %d orderer orgs and %d application orgs", len(ordererOrgs), len(allOrgs))
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

// StoreGenesisBlock stores the genesis block in a Kubernetes Secret
func (s *GenesisService) StoreGenesisBlock(ctx context.Context, genesis *v1alpha1.Genesis, genesisBlock []byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      genesis.Spec.Output.SecretName,
			Namespace: genesis.Namespace,
		},
		Data: map[string][]byte{
			genesis.Spec.Output.BlockKey: genesisBlock,
		},
	}

	// Check if secret already exists
	existingSecret := &corev1.Secret{}
	err := s.client.Get(ctx, client.ObjectKey{
		Namespace: secret.Namespace,
		Name:      secret.Name,
	}, existingSecret)

	if err != nil {
		// Secret doesn't exist, create it
		if err := s.client.Create(ctx, secret); err != nil {
			return errors.Wrap(err, "failed to create genesis block secret")
		}
	} else {
		// Secret exists, update it
		existingSecret.Data = secret.Data
		if err := s.client.Update(ctx, existingSecret); err != nil {
			return errors.Wrap(err, "failed to update genesis block secret")
		}
	}

	s.logger.Infof("Stored genesis block in secret %s/%s", secret.Namespace, secret.Name)
	return nil
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
