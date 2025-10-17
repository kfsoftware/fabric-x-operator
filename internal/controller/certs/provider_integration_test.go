package certs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// setupFabricCA starts a Fabric CA container and returns its host and port
func setupFabricCA(ctx context.Context, t *testing.T) (testcontainers.Container, string, int, []byte) {
	t.Helper()

	// Create temporary directory for CA files
	tmpDir, err := os.MkdirTemp("", "fabric-ca-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	// Fabric CA container configuration
	req := testcontainers.ContainerRequest{
		Image:        "hyperledger/fabric-ca:1.5.12",
		ExposedPorts: []string{"7054/tcp"},
		Env: map[string]string{
			"FABRIC_CA_SERVER_CA_NAME":     "ca-test",
			"FABRIC_CA_SERVER_TLS_ENABLED": "true",
			"FABRIC_CA_SERVER_DEBUG":       "true",
		},
		Cmd: []string{
			"sh", "-c",
			"fabric-ca-server start -b admin:adminpw",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("Listening on https://0.0.0.0:7054").WithStartupTimeout(60*time.Second),
			wait.ForListeningPort("7054/tcp"),
		),
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.Binds = []string{
				fmt.Sprintf("%s:/etc/hyperledger/fabric-ca-server", tmpDir),
			}
		},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start Fabric CA container: %v", err)
	}

	// Get mapped port
	mappedPort, err := container.MappedPort(ctx, "7054")
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to get container host: %v", err)
	}

	// Wait a bit for CA to fully initialize
	time.Sleep(5 * time.Second)

	// Read the CA certificate from the mounted volume
	caCertPath := filepath.Join(tmpDir, "ca-cert.pem")

	// Wait for CA cert to be created
	var caCert []byte
	for i := 0; i < 30; i++ {
		caCert, err = os.ReadFile(caCertPath)
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to read CA certificate after 30s: %v", err)
	}

	return container, host, mappedPort.Int(), caCert
}

func TestFabricCAProvider_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start Fabric CA container
	container, host, port, caCert := setupFabricCA(ctx, t)
	defer container.Terminate(ctx)

	t.Run("ProvisionSignCertificate with real Fabric CA", func(t *testing.T) {
		// Create fake Kubernetes client
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Create provider
		provider := NewFabricCAProvider()

		// Prepare request
		req := SignCertificateRequest{
			K8sClient:     k8sClient,
			MSPID:         "TestMSP",
			ComponentName: "test-component",
			Config: &FabricCAConfig{
				CAHost:       host,
				CAPort:       port,
				CAName:       "ca-test",
				EnrollID:     "admin",
				EnrollSecret: "adminpw",
				CATLS: &CATLSConfig{
					CACert: string(caCert),
				},
			},
		}

		// Provision certificate
		certData, err := provider.ProvisionSignCertificate(ctx, req)
		if err != nil {
			t.Fatalf("Failed to provision sign certificate: %v", err)
		}

		// Verify certificate data
		if certData == nil {
			t.Fatal("Certificate data is nil")
		}

		if len(certData.Certificate) == 0 {
			t.Error("Certificate is empty")
		}

		if len(certData.PrivateKey) == 0 {
			t.Error("Private key is empty")
		}

		if len(certData.CACertificate) == 0 {
			t.Error("CA certificate is empty")
		}

		if certData.Type != "sign" {
			t.Errorf("Expected type 'sign', got '%s'", certData.Type)
		}

		t.Logf("✓ Successfully enrolled sign certificate from Fabric CA")
		t.Logf("  Certificate length: %d bytes", len(certData.Certificate))
		t.Logf("  Private key length: %d bytes", len(certData.PrivateKey))
		t.Logf("  CA cert length: %d bytes", len(certData.CACertificate))
	})

	t.Run("ProvisionTLSCertificate with SANS", func(t *testing.T) {
		// Create fake Kubernetes client
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Create provider
		provider := NewFabricCAProvider()

		// Prepare request with SANS
		req := TLSCertificateRequest{
			K8sClient:     k8sClient,
			MSPID:         "TestMSP",
			ComponentName: "test-tls-component",
			DNSNames:      []string{"test.example.com", "test.local"},
			IPAddresses:   []string{"127.0.0.1"},
			Config: &FabricCAConfig{
				CAHost:       host,
				CAPort:       port,
				CAName:       "ca-test",
				EnrollID:     "admin",
				EnrollSecret: "adminpw",
				CATLS: &CATLSConfig{
					CACert: string(caCert),
				},
			},
		}

		// Provision certificate
		certData, err := provider.ProvisionTLSCertificate(ctx, req)
		if err != nil {
			t.Fatalf("Failed to provision TLS certificate: %v", err)
		}

		// Verify certificate data
		if certData == nil {
			t.Fatal("Certificate data is nil")
		}

		if len(certData.Certificate) == 0 {
			t.Error("Certificate is empty")
		}

		if len(certData.PrivateKey) == 0 {
			t.Error("Private key is empty")
		}

		if certData.Type != "tls" {
			t.Errorf("Expected type 'tls', got '%s'", certData.Type)
		}

		t.Logf("✓ Successfully enrolled TLS certificate with SANS from Fabric CA")
		t.Logf("  Certificate length: %d bytes", len(certData.Certificate))
		t.Logf("  Private key length: %d bytes", len(certData.PrivateKey))
	})
}

func TestManualProvider_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create test certificates (self-signed for testing)
	testCert := []byte(`-----BEGIN CERTIFICATE-----
MIICTjCCAdSgAwIBAgIUXa1hVyQqd5CZP8vPrR8kqYTjL+wwCgYIKoZIzj0EAwIw
aTELMAkGA1UEBhMCVVMxFzAVBgNVBAgTDk5vcnRoIENhcm9saW5hMRQwEgYDVQQK
EwtIeXBlcmxlZGdlcjEPMA0GA1UECxMGRmFicmljMRowGAYDVQQDExFmYWJyaWMt
Y2Etc2VydmVyMB4XDTI1MDExNjA0MzcwMFoXDTQwMDExMjA0MzcwMFowaTELMAkG
A1UEBhMCVVMxFzAVBgNVBAgTDk5vcnRoIENhcm9saW5hMRQwEgYDVQQKEwtIeXBl
cmxlZGdlcjEPMA0GA1UECxMGRmFicmljMRowGAYDVQQDExFmYWJyaWMtY2Etc2Vy
dmVyMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzHnF5iCzP4W0R3o0aBqrSCdZoXLu
M3nqxgVpuYPLpS6+WF8EHkC3LqPQvZsJVLpvnm5xLxYbTmGzK6SXRmNYhRpJYrJH
kLsECJGNxCvXhYEQZ+4gLH5jZFv7wLJLbr3go0IwQDAOBgNVHQ8BAf8EBAMCAQYw
DwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQU9jx0kKBFqFqLQZnXLXYZYmxU8yAw
CgYIKoZIzj0EAwIDaAAwZQIxAKrFMhqvCkD3vCHWr0+7K5BmNQqDPl8xJ8vYLGf3
wOm7LJVPHzN2jB6Q0MwFLqGJnwIwEaGMW5JYfgP0RVxJmNrKQvPWqTN0qNQ3hbQ8
oYVNFvJXhHzQ9pXJKQVfKZFxTXUZ
-----END CERTIFICATE-----`)

	testKey := []byte(`-----BEGIN EC PRIVATE KEY-----
MIGkAgEBBDDQvKZCVqLhPr7U0TmYkLqLcn3QJqVGN2WyDr1m4cxDNPZ8LQdvG0Gv
5qP7YqgHq6KgBwYFK4EEACKhZANiAATMecXmILM/hbRHejRoGqtIJ1mhcu4zeerG
BWm5g8ulLr5YXwQeQLcuo9C9mwlUum+ebnEvFhtOYbMrpJdGY1iFGkliskOQuwQI
kY3EK9eFgRBn7iAsemNkW/vAsktuveA=
-----END EC PRIVATE KEY-----`)

	testCACert := []byte(`-----BEGIN CERTIFICATE-----
MIICTjCCAdSgAwIBAgIUXa1hVyQqd5CZP8vPrR8kqYTjL+wwCgYIKoZIzj0EAwIw
aTELMAkGA1UEBhMCVVMxFzAVBgNVBAgTDk5vcnRoIENhcm9saW5hMRQwEgYDVQQK
EwtIeXBlcmxlZGdlcjEPMA0GA1UECxMGRmFicmljMRowGAYDVQQDExFmYWJyaWMt
Y2Etc2VydmVyMB4XDTI1MDExNjA0MzcwMFoXDTQwMDExMjA0MzcwMFowaTELMAkG
A1UEBhMCVVMxFzAVBgNVBAgTDk5vcnRoIENhcm9saW5hMRQwEgYDVQQKEwtIeXBl
cmxlZGdlcjEPMA0GA1UECxMGRmFicmljMRowGAYDVQQDExFmYWJyaWMtY2Etc2Vy
dmVyMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEzHnF5iCzP4W0R3o0aBqrSCdZoXLu
M3nqxgVpuYPLpS6+WF8EHkC3LqPQvZsJVLpvnm5xLxYbTmGzK6SXRmNYhRpJYrJH
kLsECJGNxCvXhYEQZ+4gLH5jZFv7wLJLbr3go0IwQDAOBgNVHQ8BAf8EBAMCAQYw
DwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQU9jx0kKBFqFqLQZnXLXYZYmxU8yAw
CgYIKoZIzj0EAwIDaAAwZQIxAKrFMhqvCkD3vCHWr0+7K5BmNQqDPl8xJ8vYLGf3
wOm7LJVPHzN2jB6Q0MwFLqGJnwIwEaGMW5JYfgP0RVxJmNrKQvPWqTN0qNQ3hbQ8
oYVNFvJXhHzQ9pXJKQVfKZFxTXUZ
-----END CERTIFICATE-----`)

	t.Run("ProvisionSignCertificate from secret", func(t *testing.T) {
		// Create secret with test certificates
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-sign-cert",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"cert.pem": testCert,
				"key.pem":  testKey,
				"ca.pem":   testCACert,
			},
		}

		// Create fake Kubernetes client with the secret
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(secret).
			Build()

		// Create provider
		provider := NewManualProvider()

		// Prepare request
		req := SignCertificateRequest{
			K8sClient:     k8sClient,
			MSPID:         "TestMSP",
			ComponentName: "test-manual-component",
			Config: &ManualConfig{
				SecretRef: &SecretRef{
					Name:      "test-sign-cert",
					Namespace: "default",
				},
			},
		}

		// Provision certificate
		certData, err := provider.ProvisionSignCertificate(ctx, req)
		if err != nil {
			t.Fatalf("Failed to provision sign certificate: %v", err)
		}

		// Verify certificate data
		if certData == nil {
			t.Fatal("Certificate data is nil")
		}

		if string(certData.Certificate) != string(testCert) {
			t.Error("Certificate doesn't match")
		}

		if string(certData.PrivateKey) != string(testKey) {
			t.Error("Private key doesn't match")
		}

		if string(certData.CACertificate) != string(testCACert) {
			t.Error("CA certificate doesn't match")
		}

		if certData.Type != "sign" {
			t.Errorf("Expected type 'sign', got '%s'", certData.Type)
		}

		t.Logf("✓ Successfully retrieved sign certificate from Kubernetes secret")
	})

	t.Run("ProvisionTLSCertificate from secret with custom keys", func(t *testing.T) {
		// Create secret with custom key names
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-tls-cert",
				Namespace: "test-namespace",
			},
			Data: map[string][]byte{
				"tls.crt":     testCert,
				"tls.key":     testKey,
				"ca-cert.pem": testCACert,
			},
		}

		// Create fake Kubernetes client with the secret
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(secret).
			Build()

		// Create provider
		provider := NewManualProvider()

		// Prepare request with custom key names
		req := TLSCertificateRequest{
			K8sClient:     k8sClient,
			MSPID:         "TestMSP",
			ComponentName: "test-manual-tls-component",
			Config: &ManualConfig{
				SecretRef: &SecretRef{
					Name:      "test-tls-cert",
					Namespace: "test-namespace",
				},
				CertKey: "tls.crt",
				KeyKey:  "tls.key",
				CAKey:   "ca-cert.pem",
			},
		}

		// Provision certificate
		certData, err := provider.ProvisionTLSCertificate(ctx, req)
		if err != nil {
			t.Fatalf("Failed to provision TLS certificate: %v", err)
		}

		// Verify certificate data
		if certData == nil {
			t.Fatal("Certificate data is nil")
		}

		if string(certData.Certificate) != string(testCert) {
			t.Error("Certificate doesn't match")
		}

		if string(certData.PrivateKey) != string(testKey) {
			t.Error("Private key doesn't match")
		}

		if certData.Type != "tls" {
			t.Errorf("Expected type 'tls', got '%s'", certData.Type)
		}

		t.Logf("✓ Successfully retrieved TLS certificate from Kubernetes secret with custom key names")
	})

	t.Run("Error when secret not found", func(t *testing.T) {
		// Create fake Kubernetes client without secrets
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Create provider
		provider := NewManualProvider()

		// Prepare request for non-existent secret
		req := SignCertificateRequest{
			K8sClient:     k8sClient,
			MSPID:         "TestMSP",
			ComponentName: "test-component",
			Config: &ManualConfig{
				SecretRef: &SecretRef{
					Name:      "non-existent-secret",
					Namespace: "default",
				},
			},
		}

		// Provision certificate (should fail)
		_, err := provider.ProvisionSignCertificate(ctx, req)
		if err == nil {
			t.Fatal("Expected error for non-existent secret, got nil")
		}

		t.Logf("✓ Correctly returned error for non-existent secret: %v", err)
	})

	t.Run("Error when certificate key missing", func(t *testing.T) {
		// Create secret without certificate
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "incomplete-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"key.pem": testKey,
			},
		}

		// Create fake Kubernetes client with incomplete secret
		scheme := runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		k8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(secret).
			Build()

		// Create provider
		provider := NewManualProvider()

		// Prepare request
		req := SignCertificateRequest{
			K8sClient:     k8sClient,
			MSPID:         "TestMSP",
			ComponentName: "test-component",
			Config: &ManualConfig{
				SecretRef: &SecretRef{
					Name:      "incomplete-secret",
					Namespace: "default",
				},
			},
		}

		// Provision certificate (should fail)
		_, err := provider.ProvisionSignCertificate(ctx, req)
		if err == nil {
			t.Fatal("Expected error for missing certificate key, got nil")
		}

		t.Logf("✓ Correctly returned error for missing certificate: %v", err)
	})
}

func TestProviderFactory_Integration(t *testing.T) {
	t.Run("GetProviderFromConfig for all providers", func(t *testing.T) {
		factory := NewProviderFactory()

		// Test Fabric CA
		fabricCAConfig := &ProviderConfig{
			Type: "fabric-ca",
			FabricCA: &FabricCAConfig{
				CAHost:       "ca.example.com",
				CAPort:       7054,
				EnrollID:     "admin",
				EnrollSecret: "password",
			},
		}
		provider, config, err := factory.GetProviderFromConfig(fabricCAConfig)
		if err != nil {
			t.Errorf("Failed to get Fabric CA provider: %v", err)
		}
		if provider.Name() != "fabric-ca" {
			t.Errorf("Expected provider name 'fabric-ca', got '%s'", provider.Name())
		}
		if _, ok := config.(*FabricCAConfig); !ok {
			t.Error("Config is not *FabricCAConfig")
		}

		// Test Manual
		manualConfig := &ProviderConfig{
			Type: "manual",
			Manual: &ManualConfig{
				SecretRef: &SecretRef{
					Name:      "test-secret",
					Namespace: "default",
				},
			},
		}
		provider, config, err = factory.GetProviderFromConfig(manualConfig)
		if err != nil {
			t.Errorf("Failed to get Manual provider: %v", err)
		}
		if provider.Name() != "manual" {
			t.Errorf("Expected provider name 'manual', got '%s'", provider.Name())
		}
		if _, ok := config.(*ManualConfig); !ok {
			t.Error("Config is not *ManualConfig")
		}

		// Test Vault
		vaultConfig := &ProviderConfig{
			Type: "vault",
			Vault: &VaultConfig{
				Address: "https://vault.example.com:8200",
				PKIPath: "pki",
				Role:    "fabric-component",
			},
		}
		provider, config, err = factory.GetProviderFromConfig(vaultConfig)
		if err != nil {
			t.Errorf("Failed to get Vault provider: %v", err)
		}
		if provider.Name() != "vault" {
			t.Errorf("Expected provider name 'vault', got '%s'", provider.Name())
		}
		if _, ok := config.(*VaultConfig); !ok {
			t.Error("Config is not *VaultConfig")
		}

		t.Logf("✓ Successfully created all three providers from factory")
	})
}

// Helper function to verify a secret was created correctly
func verifySecretInCluster(t *testing.T, k8sClient client.Client, ctx context.Context, name, namespace string) {
	t.Helper()

	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, secret)

	if err != nil {
		t.Errorf("Failed to get secret %s/%s: %v", namespace, name, err)
		return
	}

	if len(secret.Data["cert.pem"]) == 0 {
		t.Error("cert.pem is empty")
	}

	if len(secret.Data["key.pem"]) == 0 {
		t.Error("key.pem is empty")
	}

	t.Logf("✓ Secret %s/%s verified successfully", namespace, name)
}
