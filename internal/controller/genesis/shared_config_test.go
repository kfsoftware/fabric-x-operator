package genesis

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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

	return caCertPEM, tlsCACertPEM, adminCertPEM, peerCertPEM
}

func TestSharedConfigService_GenerateSharedConfig(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Generate test certificates
	ca1Cert, tls1Cert, admin1Cert, _ := generateTestCertificates(t)
	ca2Cert, tls2Cert, admin2Cert, _ := generateTestCertificates(t)
	ca3Cert, tls3Cert, admin3Cert, _ := generateTestCertificates(t)
	ca4Cert, tls4Cert, admin4Cert, _ := generateTestCertificates(t)

	// Create mock secrets for certificate references
	ca1Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca1",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":    ca1Cert,
			"tls.crt":   tls1Cert,
			"admin.crt": admin1Cert,
		},
	}

	ca2Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca2",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":    ca2Cert,
			"tls.crt":   tls2Cert,
			"admin.crt": admin2Cert,
		},
	}

	ca3Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca3",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":    ca3Cert,
			"tls.crt":   tls3Cert,
			"admin.crt": admin3Cert,
		},
	}

	ca4Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca4",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":    ca4Cert,
			"tls.crt":   tls4Cert,
			"admin.crt": admin4Cert,
		},
	}

	// Create party cert secrets
	partyCertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "party1-certs",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"cert.pem": tls1Cert,
			"sign.pem": ca1Cert,
		},
	}

	// Create fake client with secrets
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ca1Secret, ca2Secret, ca3Secret, ca4Secret, partyCertSecret).
		Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewSharedConfigService(logger, fakeClient)

	// Create test genesis with all organization types
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{
			OrdererOrganizations: []v1alpha1.OrdererOrganization{
				{
					Name:  "Org1",
					MSPID: "Org1MSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca1",
						Namespace: "default",
						Key:       "ca.crt",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca1",
						Namespace: "default",
						Key:       "tls.crt",
					},
					AdminCertRef: &v1alpha1.SecretKeyNSSelector{
						Name:      "ca1",
						Namespace: "default",
						Key:       "admin.crt",
					},
				},
				{
					Name:  "Org2",
					MSPID: "Org2MSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca2",
						Namespace: "default",
						Key:       "ca.crt",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca2",
						Namespace: "default",
						Key:       "tls.crt",
					},
					AdminCertRef: &v1alpha1.SecretKeyNSSelector{
						Name:      "ca2",
						Namespace: "default",
						Key:       "admin.crt",
					},
				},
			},
			ApplicationOrgs: []v1alpha1.ApplicationOrganization{
				{
					Name:  "AppOrg1",
					MSPID: "AppOrg1MSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca3",
						Namespace: "default",
						Key:       "ca.crt",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca3",
						Namespace: "default",
						Key:       "tls.crt",
					},
					AdminCertRef: &v1alpha1.SecretKeyNSSelector{
						Name:      "ca3",
						Namespace: "default",
						Key:       "admin.crt",
					},
				},
				{
					Name:  "AppOrg2",
					MSPID: "AppOrg2MSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca4",
						Namespace: "default",
						Key:       "ca.crt",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca4",
						Namespace: "default",
						Key:       "tls.crt",
					},
					AdminCertRef: &v1alpha1.SecretKeyNSSelector{
						Name:      "ca4",
						Namespace: "default",
						Key:       "admin.crt",
					},
				},
			},
			Parties: []v1alpha1.PartyConfig{
				{
					PartyID: 1,
					CACerts: []v1alpha1.SecretKeyNSSelector{{Name: "ca1", Namespace: "default", Key: "ca.crt"}},
					TLSCACerts: []v1alpha1.SecretKeyNSSelector{{Name: "ca1", Namespace: "default", Key: "tls.crt"}},
					RouterConfig: &v1alpha1.PartyRouterConfig{
						Host: "party1-router.default.svc.cluster.local",
						Port: 7150,
						TLSCert: v1alpha1.SecretKeyNSSelector{Name: "party1-certs", Namespace: "default", Key: "cert.pem"},
					},
					BatchersConfig: []v1alpha1.PartyBatcherConfig{
						{
							ShardID:  1,
							Host:     "party1-batcher.default.svc.cluster.local",
							Port:     7151,
							SignCert: v1alpha1.SecretKeyNSSelector{Name: "party1-certs", Namespace: "default", Key: "sign.pem"},
							TLSCert:  v1alpha1.SecretKeyNSSelector{Name: "party1-certs", Namespace: "default", Key: "cert.pem"},
						},
					},
					ConsenterConfig: &v1alpha1.PartyConsenterConfig{
						Host:     "party1-consenter.default.svc.cluster.local",
						Port:     7052,
						SignCert: v1alpha1.SecretKeyNSSelector{Name: "party1-certs", Namespace: "default", Key: "sign.pem"},
						TLSCert:  v1alpha1.SecretKeyNSSelector{Name: "party1-certs", Namespace: "default", Key: "cert.pem"},
					},
					AssemblerConfig: &v1alpha1.PartyAssemblerConfig{
						Host:    "party1-assembler.default.svc.cluster.local",
						Port:    7050,
						TLSCert: v1alpha1.SecretKeyNSSelector{Name: "party1-certs", Namespace: "default", Key: "cert.pem"},
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

	// Verify parties configuration - now expects 1 party since we explicitly defined it
	assert.Len(t, sharedConfig.PartiesConfig, 1) // 1 explicitly defined party

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
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewSharedConfigService(logger, fakeClient)

	// Create empty genesis - this should fail due to minimum party requirement
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{},
	}

	req := &SharedConfigRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	_, err = service.GenerateSharedConfig(ctx, req)

	// Should fail because at least 1 party is required
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 party is required")
}

func TestSharedConfigService_GenerateSharedConfig_Simple(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Generate test certificates for each organization
	ca1Cert, tls1Cert, _, _ := generateTestCertificates(t)
	ca2Cert, tls2Cert, _, _ := generateTestCertificates(t)
	ca3Cert, tls3Cert, _, _ := generateTestCertificates(t)
	ca4Cert, tls4Cert, _, _ := generateTestCertificates(t)

	// Create mock secrets with generated certificates
	ca1Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca1",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":  ca1Cert,
			"tls.crt": tls1Cert,
		},
	}

	ca2Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca2",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":  ca2Cert,
			"tls.crt": tls2Cert,
		},
	}

	ca3Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca3",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":  ca3Cert,
			"tls.crt": tls3Cert,
		},
	}

	ca4Secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca4",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt":  ca4Cert,
			"tls.crt": tls4Cert,
		},
	}

	// Create party cert secrets
	simplPartyCertSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-party1-certs",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"cert.pem": tls1Cert,
			"sign.pem": ca1Cert,
		},
	}

	// Create fake client with secrets
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ca1Secret, ca2Secret, ca3Secret, ca4Secret, simplPartyCertSecret).
		Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewSharedConfigService(logger, fakeClient)

	// Create a simple genesis with four orderer organizations to meet minimum requirements
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			OrdererOrganizations: []v1alpha1.OrdererOrganization{
				{
					Name:  "TestOrg1",
					MSPID: "TestOrg1MSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca1",
						Namespace: "default",
						Key:       "ca.crt",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca1",
						Namespace: "default",
						Key:       "tls.crt",
					},
				},
				{
					Name:  "TestOrg2",
					MSPID: "TestOrg2MSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca2",
						Namespace: "default",
						Key:       "ca.crt",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca2",
						Namespace: "default",
						Key:       "tls.crt",
					},
				},
				{
					Name:  "TestOrg3",
					MSPID: "TestOrg3MSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca3",
						Namespace: "default",
						Key:       "ca.crt",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca3",
						Namespace: "default",
						Key:       "tls.crt",
					},
				},
				{
					Name:  "TestOrg4",
					MSPID: "TestOrg4MSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca4",
						Namespace: "default",
						Key:       "ca.crt",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "ca4",
						Namespace: "default",
						Key:       "tls.crt",
					},
				},
			},
			Parties: []v1alpha1.PartyConfig{
				{
					PartyID:    1,
					CACerts:    []v1alpha1.SecretKeyNSSelector{{Name: "ca1", Namespace: "default", Key: "ca.crt"}},
					TLSCACerts: []v1alpha1.SecretKeyNSSelector{{Name: "ca1", Namespace: "default", Key: "tls.crt"}},
					RouterConfig: &v1alpha1.PartyRouterConfig{
						Host:    "party1-router.default.svc.cluster.local",
						Port:    7150,
						TLSCert: v1alpha1.SecretKeyNSSelector{Name: "simple-party1-certs", Namespace: "default", Key: "cert.pem"},
					},
					BatchersConfig: []v1alpha1.PartyBatcherConfig{
						{
							ShardID:  1,
							Host:     "party1-batcher.default.svc.cluster.local",
							Port:     7151,
							SignCert: v1alpha1.SecretKeyNSSelector{Name: "simple-party1-certs", Namespace: "default", Key: "sign.pem"},
							TLSCert:  v1alpha1.SecretKeyNSSelector{Name: "simple-party1-certs", Namespace: "default", Key: "cert.pem"},
						},
					},
					ConsenterConfig: &v1alpha1.PartyConsenterConfig{
						Host:     "party1-consenter.default.svc.cluster.local",
						Port:     7052,
						SignCert: v1alpha1.SecretKeyNSSelector{Name: "simple-party1-certs", Namespace: "default", Key: "sign.pem"},
						TLSCert:  v1alpha1.SecretKeyNSSelector{Name: "simple-party1-certs", Namespace: "default", Key: "cert.pem"},
					},
					AssemblerConfig: &v1alpha1.PartyAssemblerConfig{
						Host:    "party1-assembler.default.svc.cluster.local",
						Port:    7050,
						TLSCert: v1alpha1.SecretKeyNSSelector{Name: "simple-party1-certs", Namespace: "default", Key: "cert.pem"},
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
	assert.NotNil(t, sharedConfig.PartiesConfig)
	assert.NotNil(t, sharedConfig.ConsensusConfig)
	assert.NotNil(t, sharedConfig.BatchingConfig)
	assert.Len(t, sharedConfig.PartiesConfig, 1) // 1 explicitly defined party
}

func TestSharedConfigService_GenerateSharedConfig_Minimal(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewSharedConfigService(logger, fakeClient)

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
	_, err = service.GenerateSharedConfig(ctx, req)

	// Should fail because at least 1 party is required
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 party is required")
}
