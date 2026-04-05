package enrollment_test

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyperledger/fabric-ca/api"
	"github.com/kfsoftware/fabric-x-operator/internal/enrollment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	fabricCAImage = "hyperledger/fabric-ca:1.5.15"
	adminUser     = "admin"
	adminPassword = "adminpw"
)

// TestX509EnrollmentWithCA tests the complete X.509 enrollment flow with a real Fabric CA
func TestX509EnrollmentWithCA(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start Fabric CA container
	caContainer, caURL, tlsCertPath, cleanup, err := startFabricCAContainer(ctx, t)
	require.NoError(t, err, "Failed to start Fabric CA container")
	defer cleanup()

	t.Logf("Fabric CA started at %s", caURL)
	t.Logf("TLS cert at %s", tlsCertPath)

	// Read TLS cert
	tlsCert, err := os.ReadFile(tlsCertPath)
	require.NoError(t, err)

	t.Run("SuccessfulX509Enrollment", func(t *testing.T) {
		// Wait a bit for CA to be fully ready
		time.Sleep(2 * time.Second)

		req := enrollment.X509EnrollmentRequest{
			CAURL:        caURL,
			CAName:       "ca",
			EnrollID:     adminUser,
			EnrollSecret: adminPassword,
			CATLSCert:    string(tlsCert),
			MSPID:        "Org1MSP",
			CN:           adminUser,
			Hosts:        []string{"localhost", "admin.example.com"},
			Profile:      "", // Sign certificate
			Attributes:   []*api.AttributeRequest{},
		}

		resp, err := enrollment.EnrollX509(ctx, req)
		require.NoError(t, err, "X.509 enrollment should succeed")
		require.NotNil(t, resp, "Enrollment response should not be nil")

		// Validate certificate PEM
		assert.NotEmpty(t, resp.Certificate, "Certificate should not be empty")
		assert.NotEmpty(t, resp.PrivateKey, "Private key should not be empty")
		assert.NotEmpty(t, resp.CACertificate, "CA certificate should not be empty")

		// Validate raw certificate
		assert.NotNil(t, resp.CertificateRaw, "Raw certificate should not be nil")
		assert.NotNil(t, resp.PrivateKeyRaw, "Raw private key should not be nil")
		assert.NotNil(t, resp.CACertificateRaw, "Raw CA certificate should not be nil")

		// Parse and verify certificate
		block, _ := pem.Decode(resp.Certificate)
		require.NotNil(t, block, "Certificate PEM should be valid")
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err, "Certificate should be parseable")

		// Verify certificate properties
		assert.Equal(t, adminUser, cert.Subject.CommonName, "CN should match")
		assert.NotEmpty(t, cert.DNSNames, "Certificate should have DNS names")
		assert.Contains(t, cert.DNSNames, "localhost", "Certificate should contain localhost")
		assert.Contains(t, cert.DNSNames, "admin.example.com", "Certificate should contain admin.example.com")

		// Verify certificate is not expired
		now := time.Now()
		assert.True(t, cert.NotBefore.Before(now), "Certificate should be valid now")
		assert.True(t, cert.NotAfter.After(now), "Certificate should not be expired")

		t.Logf("Successfully enrolled X.509 certificate for %s", adminUser)
		t.Logf("Certificate valid from %s to %s", cert.NotBefore, cert.NotAfter)
		t.Logf("Certificate subject: %s", cert.Subject)
	})

	t.Run("TLSCertificateEnrollment", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		req := enrollment.X509EnrollmentRequest{
			CAURL:        caURL,
			CAName:       "ca",
			EnrollID:     adminUser,
			EnrollSecret: adminPassword,
			CATLSCert:    string(tlsCert),
			MSPID:        "Org1MSP",
			CN:           "tls-" + adminUser,
			Hosts:        []string{"peer0.org1.example.com", "peer0"},
			Profile:      "tls", // TLS certificate
			Attributes:   []*api.AttributeRequest{},
		}

		resp, err := enrollment.EnrollX509(ctx, req)
		require.NoError(t, err, "TLS enrollment should succeed")
		require.NotNil(t, resp, "TLS enrollment response should not be nil")

		// Validate TLS certificate
		assert.NotEmpty(t, resp.Certificate, "TLS certificate should not be empty")
		assert.NotEmpty(t, resp.PrivateKey, "TLS private key should not be empty")
		assert.NotEmpty(t, resp.CACertificate, "TLS CA certificate should not be empty")

		// Parse and verify TLS certificate
		block, _ := pem.Decode(resp.Certificate)
		require.NotNil(t, block, "TLS certificate PEM should be valid")
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err, "TLS certificate should be parseable")

		// Verify TLS certificate properties
		// Note: Fabric CA may override the CN, so we just check it's not empty
		assert.NotEmpty(t, cert.Subject.CommonName, "TLS CN should not be empty")
		assert.NotEmpty(t, cert.DNSNames, "TLS certificate should have DNS names")
		assert.Contains(t, cert.DNSNames, "peer0.org1.example.com", "TLS certificate should contain peer DNS")

		t.Logf("Successfully enrolled TLS certificate")
	})

	t.Run("EnrollmentWithAttributes", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		req := enrollment.X509EnrollmentRequest{
			CAURL:        caURL,
			CAName:       "ca",
			EnrollID:     adminUser,
			EnrollSecret: adminPassword,
			CATLSCert:    string(tlsCert),
			MSPID:        "Org1MSP",
			CN:           adminUser,
			Hosts:        []string{},
			Profile:      "",
			Attributes: []*api.AttributeRequest{
				{Name: "hf.Revoker", Optional: false},
				{Name: "hf.Registrar.Roles", Optional: false},
			},
		}

		resp, err := enrollment.EnrollX509(ctx, req)
		require.NoError(t, err, "Enrollment with attributes should succeed")
		require.NotNil(t, resp, "Response should not be nil")

		assert.NotEmpty(t, resp.Certificate, "Certificate with attributes should not be empty")

		t.Logf("Successfully enrolled certificate with attributes")
	})

	t.Run("EnrollmentWithEmailAndURI", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		req := enrollment.X509EnrollmentRequest{
			CAURL:          caURL,
			CAName:         "ca",
			EnrollID:       adminUser,
			EnrollSecret:   adminPassword,
			CATLSCert:      string(tlsCert),
			MSPID:          "Org1MSP",
			CN:             adminUser,
			Hosts:          []string{"localhost"},
			EmailAddresses: []string{"admin@example.com"},
			URIs:           []string{"https://example.com/admin"},
			Profile:        "",
			Attributes:     []*api.AttributeRequest{},
		}

		resp, err := enrollment.EnrollX509(ctx, req)
		require.NoError(t, err, "Enrollment with email and URI should succeed")
		require.NotNil(t, resp, "Response should not be nil")

		// Parse and verify certificate
		block, _ := pem.Decode(resp.Certificate)
		require.NotNil(t, block, "Certificate PEM should be valid")
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err, "Certificate should be parseable")

		// Verify SANs include emails and URIs
		assert.NotEmpty(t, cert.EmailAddresses, "Certificate should have email addresses")
		assert.NotEmpty(t, cert.URIs, "Certificate should have URIs")

		t.Logf("Successfully enrolled certificate with email and URI SANs")
		t.Logf("Email addresses: %v", cert.EmailAddresses)
		t.Logf("URIs: %v", cert.URIs)
	})

	t.Run("EnrollX509Pair_SignAndTLS", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		signReq := enrollment.X509EnrollmentRequest{
			CAURL:        caURL,
			CAName:       "ca",
			EnrollID:     adminUser,
			EnrollSecret: adminPassword,
			CATLSCert:    string(tlsCert),
			MSPID:        "Org1MSP",
			CN:           adminUser,
			Hosts:        []string{},
			Profile:      "", // Sign certificate
			Attributes:   []*api.AttributeRequest{},
		}

		tlsReq := enrollment.X509EnrollmentRequest{
			CAURL:        caURL,
			CAName:       "ca",
			EnrollID:     adminUser,
			EnrollSecret: adminPassword,
			CATLSCert:    string(tlsCert),
			MSPID:        "Org1MSP",
			CN:           "tls-" + adminUser,
			Hosts:        []string{"peer0.org1.example.com"},
			Profile:      "tls", // TLS certificate
			Attributes:   []*api.AttributeRequest{},
		}

		signResp, tlsResp, err := enrollment.EnrollX509Pair(ctx, signReq, tlsReq)
		require.NoError(t, err, "Pair enrollment should succeed")
		require.NotNil(t, signResp, "Sign response should not be nil")
		require.NotNil(t, tlsResp, "TLS response should not be nil")

		// Validate sign certificate
		assert.NotEmpty(t, signResp.Certificate, "Sign certificate should not be empty")
		assert.NotEmpty(t, signResp.PrivateKey, "Sign private key should not be empty")

		// Validate TLS certificate
		assert.NotEmpty(t, tlsResp.Certificate, "TLS certificate should not be empty")
		assert.NotEmpty(t, tlsResp.PrivateKey, "TLS private key should not be empty")

		t.Logf("Successfully enrolled sign and TLS certificate pair")
	})

	t.Run("InvalidCredentials", func(t *testing.T) {
		time.Sleep(2 * time.Second)

		req := enrollment.X509EnrollmentRequest{
			CAURL:        caURL,
			CAName:       "ca",
			EnrollID:     "nonexistent",
			EnrollSecret: "wrongpassword",
			CATLSCert:    string(tlsCert),
			MSPID:        "Org1MSP",
			CN:           "nonexistent",
			Hosts:        []string{},
			Profile:      "",
			Attributes:   []*api.AttributeRequest{},
		}

		resp, err := enrollment.EnrollX509(ctx, req)
		assert.Error(t, err, "Enrollment with invalid credentials should fail")
		assert.Nil(t, resp, "Response should be nil on error")
		assert.Contains(t, err.Error(), "failed to enroll user", "Error should mention enrollment failure")
	})

	t.Run("InvalidCAURL", func(t *testing.T) {
		req := enrollment.X509EnrollmentRequest{
			CAURL:        "https://invalid.example.com:9999",
			CAName:       "ca",
			EnrollID:     adminUser,
			EnrollSecret: adminPassword,
			CATLSCert:    string(tlsCert),
			MSPID:        "Org1MSP",
			CN:           adminUser,
			Hosts:        []string{},
			Profile:      "",
			Attributes:   []*api.AttributeRequest{},
		}

		resp, err := enrollment.EnrollX509(ctx, req)
		assert.Error(t, err, "Enrollment with invalid CA URL should fail")
		assert.Nil(t, resp, "Response should be nil on error")
	})

	t.Run("InvalidTLSCert", func(t *testing.T) {
		req := enrollment.X509EnrollmentRequest{
			CAURL:        caURL,
			CAName:       "ca",
			EnrollID:     adminUser,
			EnrollSecret: adminPassword,
			CATLSCert:    "-----BEGIN CERTIFICATE-----\nINVALID\n-----END CERTIFICATE-----",
			MSPID:        "Org1MSP",
			CN:           adminUser,
			Hosts:        []string{},
			Profile:      "",
			Attributes:   []*api.AttributeRequest{},
		}

		resp, err := enrollment.EnrollX509(ctx, req)
		assert.Error(t, err, "Enrollment with invalid TLS cert should fail")
		assert.Nil(t, resp, "Response should be nil on error")
	})

	// Test container cleanup
	t.Run("CleanupContainer", func(t *testing.T) {
		// Get container logs before cleanup
		logs, err := caContainer.Logs(ctx)
		if err != nil {
			t.Logf("Failed to get container logs: %v", err)
		} else {
			defer logs.Close()
			logBytes, _ := io.ReadAll(logs)
			t.Logf("CA Container logs:\n%s", string(logBytes))
		}
	})
}

// TestX509EnrollmentPair_OnlySign tests enrolling only sign certificate
func TestX509EnrollmentPair_OnlySign(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start Fabric CA container
	_, caURL, tlsCertPath, cleanup, err := startFabricCAContainer(ctx, t)
	require.NoError(t, err, "Failed to start Fabric CA container")
	defer cleanup()

	// Read TLS cert
	tlsCert, err := os.ReadFile(tlsCertPath)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	signReq := enrollment.X509EnrollmentRequest{
		CAURL:        caURL,
		CAName:       "ca",
		EnrollID:     adminUser,
		EnrollSecret: adminPassword,
		CATLSCert:    string(tlsCert),
		MSPID:        "Org1MSP",
		CN:           adminUser,
		Hosts:        []string{},
		Profile:      "",
		Attributes:   []*api.AttributeRequest{},
	}

	// Empty TLS request (no TLS enrollment)
	tlsReq := enrollment.X509EnrollmentRequest{}

	signResp, tlsResp, err := enrollment.EnrollX509Pair(ctx, signReq, tlsReq)
	require.NoError(t, err, "Sign-only enrollment should succeed")
	require.NotNil(t, signResp, "Sign response should not be nil")
	assert.Nil(t, tlsResp, "TLS response should be nil when not requested")

	// Validate sign certificate
	assert.NotEmpty(t, signResp.Certificate, "Sign certificate should not be empty")
}

// startFabricCAContainer starts a Fabric CA container and returns the container, URL, TLS cert path, and cleanup function
func startFabricCAContainer(ctx context.Context, t *testing.T) (testcontainers.Container, string, string, func(), error) {
	// Create temporary directory for CA files
	tmpDir, err := os.MkdirTemp("", "fabric-ca-test-*")
	if err != nil {
		return nil, "", "", nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	req := testcontainers.ContainerRequest{
		Image:        fabricCAImage,
		ExposedPorts: []string{"7054/tcp"},
		Env: map[string]string{
			"FABRIC_CA_HOME":           "/etc/hyperledger/fabric-ca-server",
			"FABRIC_CA_SERVER_CA_NAME": "ca",
			"FABRIC_CA_SERVER_PORT":    "7054",
			"FABRIC_CA_SERVER_DEBUG":   "true",
		},
		Cmd: []string{
			"sh", "-c",
			"fabric-ca-server start -b admin:adminpw --tls.enabled -d",
		},
		WaitingFor: wait.ForLog("Listening on https://0.0.0.0:7054").
			WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, "", "", nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Get mapped port
	mappedPort, err := container.MappedPort(ctx, "7054")
	if err != nil {
		container.Terminate(ctx)
		os.RemoveAll(tmpDir)
		return nil, "", "", nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	// Get host
	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		os.RemoveAll(tmpDir)
		return nil, "", "", nil, fmt.Errorf("failed to get host: %w", err)
	}

	caURL := fmt.Sprintf("https://%s:%s", host, mappedPort.Port())

	// Wait for CA to be fully ready
	time.Sleep(5 * time.Second)

	// Get TLS certificate from container
	tlsCertPath, err := getTLSCertFromContainer(ctx, container, tmpDir)
	if err != nil {
		container.Terminate(ctx)
		os.RemoveAll(tmpDir)
		return nil, "", "", nil, fmt.Errorf("failed to get TLS cert: %w", err)
	}

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
		os.RemoveAll(tmpDir)
	}

	return container, caURL, tlsCertPath, cleanup, nil
}

// getTLSCertFromContainer retrieves the TLS certificate from the container and saves it to a file
func getTLSCertFromContainer(ctx context.Context, container testcontainers.Container, tmpDir string) (string, error) {
	// Wait a bit more for TLS cert to be generated
	time.Sleep(2 * time.Second)

	// Read the TLS cert from the container
	exitCode, reader, err := container.Exec(ctx, []string{"cat", "/etc/hyperledger/fabric-ca-server/tls-cert.pem"})
	if err != nil {
		return "", fmt.Errorf("failed to exec cat command: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("cat command returned non-zero exit code: %d", exitCode)
	}

	tlsCertBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read TLS cert: %w", err)
	}

	if len(tlsCertBytes) == 0 {
		return "", fmt.Errorf("TLS cert is empty")
	}

	// Trim whitespace and control characters from the cert content
	// The exec output may include extra characters
	tlsCert := []byte(string(tlsCertBytes))
	// Find the start of the PEM block
	pemStart := []byte("-----BEGIN CERTIFICATE-----")
	startIdx := 0
	for i := range tlsCert {
		if i+len(pemStart) <= len(tlsCert) {
			if string(tlsCert[i:i+len(pemStart)]) == string(pemStart) {
				startIdx = i
				break
			}
		}
	}
	tlsCert = tlsCert[startIdx:]

	// Save to file
	tlsCertPath := filepath.Join(tmpDir, "tls-cert.pem")
	if err := os.WriteFile(tlsCertPath, tlsCert, 0644); err != nil {
		return "", fmt.Errorf("failed to write TLS cert file: %w", err)
	}

	return tlsCertPath, nil
}
