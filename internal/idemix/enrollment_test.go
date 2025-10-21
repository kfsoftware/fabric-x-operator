package idemix_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kfsoftware/fabric-x-operator/internal/idemix"
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

// TestIdemixEnrollmentWithCA tests the complete idemix enrollment flow with a real Fabric CA
func TestIdemixEnrollmentWithCA(t *testing.T) {
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

	// Test idemix enrollment with admin user (pre-registered)
	t.Run("SuccessfulIdemixEnrollment", func(t *testing.T) {
		// Create temporary MSP directory for test
		mspDir, err := os.MkdirTemp("", "test-idemix-msp-*")
		require.NoError(t, err)
		defer os.RemoveAll(mspDir)

		// Wait a bit more for CA to be fully ready
		time.Sleep(3 * time.Second)

		// Debug: check TLS cert content
		certContent, err := os.ReadFile(tlsCertPath)
		require.NoError(t, err)
		t.Logf("TLS cert content length: %d bytes", len(certContent))
		t.Logf("TLS cert first 100 chars: %s", string(certContent[:min(100, len(certContent))]))

		req := idemix.EnrollmentRequest{
			CAURL:        caURL,
			CAName:       "ca",
			EnrollID:     adminUser,
			EnrollSecret: adminPassword,
			CACertPath:   tlsCertPath, // Use TLS cert for verification
			MSPDir:       mspDir,
		}

		resp, err := idemix.Enroll(req)
		require.NoError(t, err, "Idemix enrollment should succeed")
		require.NotNil(t, resp, "Enrollment response should not be nil")

		// Validate SignerConfig
		assert.NotNil(t, resp.SignerConfig, "SignerConfig should not be nil")
		assert.NotEmpty(t, resp.SignerConfig.GetCred(), "Credential should not be empty")
		assert.NotEmpty(t, resp.SignerConfig.GetSk(), "Secret key should not be empty")
		assert.Equal(t, adminUser, resp.SignerConfig.GetEnrollmentID(), "Enrollment ID should match")
		assert.NotEmpty(t, resp.SignerConfig.GetCredentialRevocationInformation(), "CRI should not be empty")

		// Validate idemix config path
		assert.NotEmpty(t, resp.IdemixConfigPath, "Idemix config path should not be empty")
		assert.DirExists(t, resp.IdemixConfigPath, "Idemix config directory should exist")

		t.Logf("Successfully enrolled idemix credential for %s", adminUser)
		t.Logf("Credential size: %d bytes", len(resp.SignerConfig.GetCred()))
		t.Logf("Secret key size: %d bytes", len(resp.SignerConfig.GetSk()))
		t.Logf("Idemix config path: %s", resp.IdemixConfigPath)

		// Verify files are created
		files, err := os.ReadDir(resp.IdemixConfigPath)
		require.NoError(t, err)
		t.Logf("Files in idemix config dir: %d", len(files))
		for _, f := range files {
			t.Logf("  - %s", f.Name())
		}
	})

	t.Run("InvalidCredentials", func(t *testing.T) {
		mspDir, err := os.MkdirTemp("", "test-idemix-msp-*")
		require.NoError(t, err)
		defer os.RemoveAll(mspDir)

		req := idemix.EnrollmentRequest{
			CAURL:         caURL,
			CAName:        "ca",
			EnrollID:      "nonexistent",
			EnrollSecret:  "wrongpassword",
			SkipTLSVerify: true,
			MSPDir:        mspDir,
		}

		resp, err := idemix.Enroll(req)
		assert.Error(t, err, "Enrollment with invalid credentials should fail")
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
			"fabric-ca-server start -b admin:adminpw --tls.enabled --idemix.curve gurvy.Bn254 -d",
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
