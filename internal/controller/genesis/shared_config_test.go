package genesis

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// generateTestCertificates creates valid test certificates using Go's crypto library
func generateTestCertificates(t *testing.T) ([]byte, []byte, []byte, []byte) {
	// Generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Create CA certificate template
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create CA certificate
	caCertBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Create TLS CA certificate template
	tlsCATemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Test TLS CA"},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create TLS CA certificate
	tlsCACertBytes, err := x509.CreateCertificate(rand.Reader, tlsCATemplate, tlsCATemplate, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Create admin certificate template
	adminTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			Country:      []string{"US"},
			CommonName:   "admin",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	// Create admin certificate
	adminCertBytes, err := x509.CreateCertificate(rand.Reader, adminTemplate, caTemplate, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Create peer certificate template
	peerTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(4),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			Country:      []string{"US"},
			CommonName:   "peer",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	// Create peer certificate
	peerCertBytes, err := x509.CreateCertificate(rand.Reader, peerTemplate, caTemplate, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Encode certificates to PEM format
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertBytes})
	tlsCACertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsCACertBytes})
	adminCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: adminCertBytes})
	peerCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: peerCertBytes})

	// Encode to base64 for external orgs
	caCertBase64 := base64.StdEncoding.EncodeToString(caCertPEM)
	tlsCACertBase64 := base64.StdEncoding.EncodeToString(tlsCACertPEM)
	adminCertBase64 := base64.StdEncoding.EncodeToString(adminCertPEM)
	peerCertBase64 := base64.StdEncoding.EncodeToString(peerCertPEM)

	return []byte(caCertBase64), []byte(tlsCACertBase64), []byte(adminCertBase64), []byte(peerCertBase64)
}

func TestSharedConfigService_GenerateSharedConfig(t *testing.T) {
	logger := logrus.New()
	service := NewSharedConfigService(logger)

	// Generate test certificates
	caCert, tlsCACert, adminCert, peerCert := generateTestCertificates(t)

	// Create test genesis with all organization types
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{
			InternalOrgs: []v1alpha1.InternalOrganization{
				{
					Name:  "Org1",
					MSPID: "Org1MSP",
					CAReference: v1alpha1.CAReference{
						Name:      "ca1",
						Namespace: "default",
					},
					AdminIdentity:   "admin",
					OrdererIdentity: "orderer",
				},
			},
			ExternalOrgs: []v1alpha1.ExternalOrganization{
				{
					Name:        "Org2",
					MSPID:       "Org2MSP",
					SignCert:    string(caCert),
					TLSCert:     string(tlsCACert),
					AdminCert:   string(adminCert),
					OrdererCert: string(peerCert),
				},
			},
			ApplicationOrgs: []v1alpha1.ApplicationOrganization{
				{
					Name:  "AppOrg1",
					MSPID: "AppOrg1MSP",
					Type:  "internal",
					Internal: &v1alpha1.InternalApplicationOrg{
						CAReference: v1alpha1.CAReference{
							Name:      "ca2",
							Namespace: "default",
						},
						AdminIdentity: "admin",
						PeerIdentity:  "peer",
					},
				},
				{
					Name:  "AppOrg2",
					MSPID: "AppOrg2MSP",
					Type:  "external",
					External: &v1alpha1.ExternalApplicationOrg{
						SignCACert: string(caCert),
						TLSCACert:  string(tlsCACert),
						AdminCert:  string(adminCert),
						PeerCert:   string(peerCert),
					},
				},
			},
		},
	}

	req := &SharedConfigRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	sharedConfig, err := service.GenerateSharedConfig(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, sharedConfig)

	// Verify parties configuration
	assert.Len(t, sharedConfig.PartiesConfig, 4) // 1 internal + 1 external + 2 application orgs

	// Verify party IDs are assigned correctly
	partyIDs := make(map[uint32]bool)
	for _, party := range sharedConfig.PartiesConfig {
		assert.Greater(t, party.PartyID, uint32(0))
		assert.False(t, partyIDs[party.PartyID], "Party ID should be unique")
		partyIDs[party.PartyID] = true

		// Verify CA certificates
		assert.NotEmpty(t, party.CACerts)
		assert.NotEmpty(t, party.TLSCACerts)

		// Verify node configurations
		assert.NotNil(t, party.RouterConfig)
		assert.NotNil(t, party.ConsenterConfig)
		assert.NotNil(t, party.AssemblerConfig)
		assert.NotEmpty(t, party.BatchersConfig)

		// Verify router config
		assert.NotEmpty(t, party.RouterConfig.Host)
		assert.Greater(t, party.RouterConfig.Port, uint32(0))
		assert.NotEmpty(t, party.RouterConfig.TlsCert)

		// Verify batcher config
		for _, batcher := range party.BatchersConfig {
			assert.Greater(t, batcher.ShardID, uint32(0))
			assert.NotEmpty(t, batcher.Host)
			assert.Greater(t, batcher.Port, uint32(0))
			assert.NotEmpty(t, batcher.SignCert)
			assert.NotEmpty(t, batcher.TlsCert)
		}

		// Verify consenter config
		assert.NotEmpty(t, party.ConsenterConfig.Host)
		assert.Greater(t, party.ConsenterConfig.Port, uint32(0))
		assert.NotEmpty(t, party.ConsenterConfig.SignCert)
		assert.NotEmpty(t, party.ConsenterConfig.TlsCert)

		// Verify assembler config
		assert.NotEmpty(t, party.AssemblerConfig.Host)
		assert.Greater(t, party.AssemblerConfig.Port, uint32(0))
		assert.NotEmpty(t, party.AssemblerConfig.TlsCert)
	}

	// Verify consensus configuration
	assert.NotNil(t, sharedConfig.ConsensusConfig)
	assert.NotNil(t, sharedConfig.ConsensusConfig.SmartBFTConfig)

	smartBFT := sharedConfig.ConsensusConfig.SmartBFTConfig
	assert.Greater(t, smartBFT.RequestBatchMaxCount, uint64(0))
	assert.Greater(t, smartBFT.RequestBatchMaxBytes, uint64(0))
	assert.NotEmpty(t, smartBFT.RequestBatchMaxInterval)
	assert.Greater(t, smartBFT.RequestPoolSize, uint64(0))
	assert.NotEmpty(t, smartBFT.RequestForwardTimeout)
	assert.NotEmpty(t, smartBFT.RequestComplainTimeout)
	assert.NotEmpty(t, smartBFT.RequestAutoRemoveTimeout)
	assert.NotEmpty(t, smartBFT.ViewChangeResendInterval)
	assert.NotEmpty(t, smartBFT.ViewChangeTimeout)
	assert.NotEmpty(t, smartBFT.LeaderHeartbeatTimeout)
	assert.Greater(t, smartBFT.LeaderHeartbeatCount, uint64(0))
	assert.Greater(t, smartBFT.NumOfTicksBehindBeforeSyncing, uint64(0))
	assert.NotEmpty(t, smartBFT.CollectTimeout)
	assert.True(t, smartBFT.SyncOnStart)
	assert.False(t, smartBFT.SpeedUpViewChange)
	assert.True(t, smartBFT.LeaderRotation)
	assert.Greater(t, smartBFT.DecisionsPerLeader, uint64(0))
	assert.Greater(t, smartBFT.RequestMaxBytes, uint64(0))
	assert.NotEmpty(t, smartBFT.RequestPoolSubmitTimeout)

	// Verify batching configuration
	assert.NotNil(t, sharedConfig.BatchingConfig)
	assert.NotNil(t, sharedConfig.BatchingConfig.BatchTimeouts)
	assert.NotNil(t, sharedConfig.BatchingConfig.BatchSize)

	batchTimeouts := sharedConfig.BatchingConfig.BatchTimeouts
	assert.NotEmpty(t, batchTimeouts.BatchCreationTimeout)
	assert.NotEmpty(t, batchTimeouts.FirstStrikeThreshold)
	assert.NotEmpty(t, batchTimeouts.SecondStrikeThreshold)
	assert.NotEmpty(t, batchTimeouts.AutoRemoveTimeout)

	batchSize := sharedConfig.BatchingConfig.BatchSize
	assert.Greater(t, batchSize.MaxMessageCount, uint32(0))
	assert.Greater(t, batchSize.AbsoluteMaxBytes, uint32(0))
	assert.Greater(t, batchSize.PreferredMaxBytes, uint32(0))

	assert.Greater(t, sharedConfig.BatchingConfig.RequestMaxBytes, uint64(0))
}

func TestSharedConfigService_GenerateSharedConfig_EmptyGenesis(t *testing.T) {
	logger := logrus.New()
	service := NewSharedConfigService(logger)

	// Create empty genesis - this should fail due to minimum party requirement
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{},
	}

	req := &SharedConfigRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	_, err := service.GenerateSharedConfig(ctx, req)

	// Should fail because at least 1 party is required
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 party is required")
}

func TestSharedConfigService_GenerateSharedConfig_InvalidOrgType(t *testing.T) {
	logger := logrus.New()
	service := NewSharedConfigService(logger)

	// Create genesis with invalid organization type
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{
			ApplicationOrgs: []v1alpha1.ApplicationOrganization{
				{
					Name:  "InvalidOrg",
					MSPID: "InvalidOrgMSP",
					Type:  "invalid",
				},
			},
		},
	}

	req := &SharedConfigRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	_, err := service.GenerateSharedConfig(ctx, req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create party config for application org InvalidOrg")
}

func TestSharedConfigService_GenerateSharedConfig_MissingInternalConfig(t *testing.T) {
	logger := logrus.New()
	service := NewSharedConfigService(logger)

	// Create genesis with internal org type but missing internal config
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{
			ApplicationOrgs: []v1alpha1.ApplicationOrganization{
				{
					Name:  "InvalidOrg",
					MSPID: "InvalidOrgMSP",
					Type:  "internal",
					// Missing Internal field
				},
			},
		},
	}

	req := &SharedConfigRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	_, err := service.GenerateSharedConfig(ctx, req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "internal configuration is required")
}

func TestSharedConfigService_GenerateSharedConfig_MissingExternalConfig(t *testing.T) {
	logger := logrus.New()
	service := NewSharedConfigService(logger)

	// Create genesis with external org type but missing external config
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{
			ApplicationOrgs: []v1alpha1.ApplicationOrganization{
				{
					Name:  "InvalidOrg",
					MSPID: "InvalidOrgMSP",
					Type:  "external",
					// Missing External field
				},
			},
		},
	}

	req := &SharedConfigRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	_, err := service.GenerateSharedConfig(ctx, req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create party config for application org InvalidOrg")
}

func TestSharedConfigService_GenerateSharedConfig_Simple(t *testing.T) {
	logger := logrus.New()
	service := NewSharedConfigService(logger)

	// Create a simple genesis with four external organizations to meet minimum requirements
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			ExternalOrgs: []v1alpha1.ExternalOrganization{
				{
					Name:        "TestOrg1",
					MSPID:       "TestOrg1MSP",
					SignCert:    "dGVzdC1jZXJ0",             // base64 encoded "test-cert"
					TLSCert:     "dGVzdC10bHMtY2VydA==",     // base64 encoded "test-tls-cert"
					AdminCert:   "dGVzdC1hZG1pbi1jZXJ0",     // base64 encoded "test-admin-cert"
					OrdererCert: "dGVzdC1vcmRlcmVyLWNlcnQ=", // base64 encoded "test-orderer-cert"
				},
				{
					Name:        "TestOrg2",
					MSPID:       "TestOrg2MSP",
					SignCert:    "dGVzdC1jZXJ0",             // base64 encoded "test-cert"
					TLSCert:     "dGVzdC10bHMtY2VydA==",     // base64 encoded "test-tls-cert"
					AdminCert:   "dGVzdC1hZG1pbi1jZXJ0",     // base64 encoded "test-admin-cert"
					OrdererCert: "dGVzdC1vcmRlcmVyLWNlcnQ=", // base64 encoded "test-orderer-cert"
				},
				{
					Name:        "TestOrg3",
					MSPID:       "TestOrg3MSP",
					SignCert:    "dGVzdC1jZXJ0",             // base64 encoded "test-cert"
					TLSCert:     "dGVzdC10bHMtY2VydA==",     // base64 encoded "test-tls-cert"
					AdminCert:   "dGVzdC1hZG1pbi1jZXJ0",     // base64 encoded "test-admin-cert"
					OrdererCert: "dGVzdC1vcmRlcmVyLWNlcnQ=", // base64 encoded "test-orderer-cert"
				},
				{
					Name:        "TestOrg4",
					MSPID:       "TestOrg4MSP",
					SignCert:    "dGVzdC1jZXJ0",             // base64 encoded "test-cert"
					TLSCert:     "dGVzdC10bHMtY2VydA==",     // base64 encoded "test-tls-cert"
					AdminCert:   "dGVzdC1hZG1pbi1jZXJ0",     // base64 encoded "test-admin-cert"
					OrdererCert: "dGVzdC1vcmRlcmVyLWNlcnQ=", // base64 encoded "test-orderer-cert"
				},
			},
		},
	}

	req := &SharedConfigRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	sharedConfig, err := service.GenerateSharedConfig(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, sharedConfig)
	assert.NotNil(t, sharedConfig.PartiesConfig)
	assert.NotNil(t, sharedConfig.ConsensusConfig)
	assert.NotNil(t, sharedConfig.BatchingConfig)
	assert.Len(t, sharedConfig.PartiesConfig, 4)
}

func TestSharedConfigService_GenerateSharedConfig_Minimal(t *testing.T) {
	logger := logrus.New()
	service := NewSharedConfigService(logger)

	// Create a minimal genesis with no organizations - this should fail due to minimum party requirement
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			// No organizations - this should fail due to minimum party requirement
		},
	}

	req := &SharedConfigRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	_, err := service.GenerateSharedConfig(ctx, req)

	// Should fail because at least 1 party is required
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 party is required")
}
