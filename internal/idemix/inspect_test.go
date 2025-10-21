package idemix_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kfsoftware/fabric-x-operator/internal/idemix"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestInspectSignerConfig shows the structure of the SignerConfig
func TestInspectSignerConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create temp dir
	tmpDir, err := os.MkdirTemp("", "fabric-ca-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Start CA
	req := testcontainers.ContainerRequest{
		Image:        "hyperledger/fabric-ca:1.5.15",
		ExposedPorts: []string{"7054/tcp"},
		Env: map[string]string{
			"FABRIC_CA_HOME":           "/etc/hyperledger/fabric-ca-server",
			"FABRIC_CA_SERVER_CA_NAME": "ca",
			"FABRIC_CA_SERVER_PORT":    "7054",
		},
		Cmd: []string{
			"sh", "-c",
			"fabric-ca-server start -b admin:adminpw --tls.enabled --idemix.curve gurvy.Bn254 -d",
		},
		WaitingFor: wait.ForLog("Listening on https://0.0.0.0:7054").
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer container.Terminate(ctx)

	// Wait for CA
	time.Sleep(7 * time.Second)

	// Get TLS cert
	exitCode, reader, err := container.Exec(ctx, []string{"cat", "/etc/hyperledger/fabric-ca-server/tls-cert.pem"})
	if err != nil || exitCode != 0 {
		t.Fatal("Failed to get TLS cert")
	}

	tlsCertBytes, _ := io.ReadAll(reader)
	pemStart := []byte("-----BEGIN CERTIFICATE-----")
	startIdx := 0
	for i := range tlsCertBytes {
		if i+len(pemStart) <= len(tlsCertBytes) {
			if string(tlsCertBytes[i:i+len(pemStart)]) == string(pemStart) {
				startIdx = i
				break
			}
		}
	}
	tlsCert := tlsCertBytes[startIdx:]
	tlsCertPath := filepath.Join(tmpDir, "tls-cert.pem")
	os.WriteFile(tlsCertPath, tlsCert, 0644)

	// Get CA URL
	mappedPort, _ := container.MappedPort(ctx, "7054")
	host, _ := container.Host(ctx)
	caURL := fmt.Sprintf("https://%s:%s", host, mappedPort.Port())

	// Enroll
	mspDir, _ := os.MkdirTemp("", "test-idemix-msp-*")
	defer os.RemoveAll(mspDir)

	enrollReq := idemix.EnrollmentRequest{
		CAURL:        caURL,
		CAName:       "ca",
		EnrollID:     "admin",
		EnrollSecret: "adminpw",
		CACertPath:   tlsCertPath,
		MSPDir:       mspDir,
	}

	resp, err := idemix.Enroll(enrollReq)
	if err != nil {
		t.Fatalf("Enrollment failed: %v", err)
	}

	// Print SignerConfig as formatted JSON
	signerConfigJSON, _ := json.MarshalIndent(resp.SignerConfig, "", "  ")
	t.Logf("\n=== SignerConfig JSON ===\n%s", string(signerConfigJSON))

	t.Logf("\n=== Individual Field Sizes ===")
	t.Logf("Cred size: %d bytes", len(resp.SignerConfig.GetCred()))
	t.Logf("Sk size: %d bytes", len(resp.SignerConfig.GetSk()))
	t.Logf("CRI size: %d bytes", len(resp.SignerConfig.GetCredentialRevocationInformation()))
	t.Logf("EnrollmentID: %s", resp.SignerConfig.GetEnrollmentID())
	t.Logf("OU: %s", resp.SignerConfig.GetOrganizationalUnitIdentifier())
	t.Logf("Role: %d", resp.SignerConfig.GetRole())
	t.Logf("CurveID: %s", resp.SignerConfig.CurveID)
	t.Logf("RevocationHandle: %s", resp.SignerConfig.RevocationHandle)

	// Show first 100 bytes of Cred as hex
	cred := resp.SignerConfig.GetCred()
	if len(cred) > 100 {
		t.Logf("\nFirst 100 bytes of Cred (hex): %x...", cred[:100])
	} else {
		t.Logf("\nCred (hex): %x", cred)
	}

	// Show Sk as hex
	sk := resp.SignerConfig.GetSk()
	t.Logf("Sk (hex): %x", sk)
}
