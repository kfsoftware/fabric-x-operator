package controller_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller"
)

const (
	fabricCAImage = "hyperledger/fabric-ca:1.5.15"
	adminUser     = "admin"
	adminPassword = "adminpw"
	testTimeout   = 300 * time.Second
	pollInterval  = 2 * time.Second
)

// TestIdentityControllerE2E tests the complete Identity controller workflow with a real Fabric CA
func TestIdentityControllerE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E integration test in short mode")
	}

	// Setup logging
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Start test environment and controller
	testEnv, k8sClient, stopController := setupTestEnvironment(t, ctx)
	defer stopController()
	defer testEnv.Stop()

	// Start Fabric CA container
	caContainer, caURL, tlsCertPath, cleanupCA := startFabricCAContainer(t, ctx)
	defer cleanupCA()

	t.Logf("Fabric CA started at %s", caURL)
	t.Logf("TLS cert at %s", tlsCertPath)

	// Read TLS cert
	tlsCert, err := os.ReadFile(tlsCertPath)
	require.NoError(t, err)

	// Wait for CA to be fully ready
	time.Sleep(5 * time.Second)

	// Run E2E test scenarios
	t.Run("CreateCA_SignEnrollment_X509", func(t *testing.T) {
		testSignEnrollmentX509(t, ctx, k8sClient, caURL, string(tlsCert))
	})

	t.Run("CreateCA_SignAndTLS_X509", func(t *testing.T) {
		testSignAndTLSEnrollmentX509(t, ctx, k8sClient, caURL, string(tlsCert))
	})

	t.Run("CreateCA_Idemix_Enrollment", func(t *testing.T) {
		testIdemixEnrollment(t, ctx, k8sClient, caURL, string(tlsCert))
	})

	t.Run("SecretCleanupOnDeletion", func(t *testing.T) {
		testSecretCleanupOnDeletion(t, ctx, k8sClient, caURL, string(tlsCert))
	})

	// Show CA logs
	t.Run("ShowCALogs", func(t *testing.T) {
		logs, err := caContainer.Logs(ctx)
		if err != nil {
			t.Logf("Failed to get CA logs: %v", err)
		} else {
			defer logs.Close()
			logBytes, _ := io.ReadAll(logs)
			t.Logf("CA Container logs (last 5000 chars):\n%s", truncateString(string(logBytes), 5000))
		}
	})
}

// testSignEnrollmentX509 tests sign certificate enrollment
func testSignEnrollmentX509(t *testing.T, ctx context.Context, k8sClient client.Client, caURL, tlsCert string) {
	namespace := "default"

	// Create CA TLS secret
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca-tls-crypto",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte(tlsCert),
		},
	}
	err := k8sClient.Create(ctx, caSecret)
	require.NoError(t, err, "Failed to create CA TLS secret")

	// Create enrollment secret
	enrollSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-enroll-secret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"password": []byte(adminPassword),
		},
	}
	err = k8sClient.Create(ctx, enrollSecret)
	require.NoError(t, err, "Failed to create enrollment secret")

	// Create CA resource
	ca := &fabricxv1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ca",
			Namespace: namespace,
		},
		Spec: fabricxv1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca",
			Version: "1.5.15",
			CA: fabricxv1alpha1.FabricCAItemConf{
				Name: "ca",
			},
		},
	}
	err = k8sClient.Create(ctx, ca)
	require.NoError(t, err, "Failed to create CA resource")

	// Create Identity resource with sign enrollment only
	identity := &fabricxv1alpha1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-identity-sign",
			Namespace: namespace,
		},
		Spec: fabricxv1alpha1.IdentitySpec{
			Type:  "user",
			MspID: "Org1MSP",
			Enrollment: &fabricxv1alpha1.IdentityEnrollment{
				CARef: fabricxv1alpha1.IdentityCARef{
					Name:      "test-ca",
					Namespace: namespace,
				},
				EnrollID: adminUser,
				EnrollSecretRef: fabricxv1alpha1.SecretKeyNSSelector{
					Name:      "test-enroll-secret",
					Key:       "password",
					Namespace: namespace,
				},
				EnrollTLS: false, // Only sign certificate
			},
			Output: fabricxv1alpha1.IdentityOutput{
				SecretPrefix: "test-identity-sign",
				Namespace:    namespace,
			},
		},
	}
	err = k8sClient.Create(ctx, identity)
	require.NoError(t, err, "Failed to create Identity resource")

	// Wait for identity to be ready
	err = waitForIdentityReady(ctx, k8sClient, types.NamespacedName{
		Name:      "test-identity-sign",
		Namespace: namespace,
	}, 60*time.Second)
	require.NoError(t, err, "Identity should become ready")

	// Verify secrets were created
	signCertSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Name:      "test-identity-sign-sign-cert",
		Namespace: namespace,
	}, signCertSecret)
	require.NoError(t, err, "Sign cert secret should exist")
	assert.NotEmpty(t, signCertSecret.Data["cert.pem"], "Sign cert should not be empty")

	signKeySecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Name:      "test-identity-sign-sign-key",
		Namespace: namespace,
	}, signKeySecret)
	require.NoError(t, err, "Sign key secret should exist")
	assert.NotEmpty(t, signKeySecret.Data["cert.pem"], "Sign key should not be empty")

	// Verify combined secret
	combinedSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Name:      "test-identity-sign-cert",
		Namespace: namespace,
	}, combinedSecret)
	require.NoError(t, err, "Combined sign secret should exist")
	assert.NotEmpty(t, combinedSecret.Data["cert.pem"], "Combined cert should not be empty")
	assert.NotEmpty(t, combinedSecret.Data["key.pem"], "Combined key should not be empty")
	assert.NotEmpty(t, combinedSecret.Data["ca.pem"], "Combined CA cert should not be empty")

	t.Logf("✅ Sign enrollment completed successfully")
}

// testSignAndTLSEnrollmentX509 tests dual enrollment (sign + TLS)
func testSignAndTLSEnrollmentX509(t *testing.T, ctx context.Context, k8sClient client.Client, caURL, tlsCert string) {
	namespace := "default"

	// Create Identity resource with both sign and TLS enrollment
	identity := &fabricxv1alpha1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-identity-dual",
			Namespace: namespace,
		},
		Spec: fabricxv1alpha1.IdentitySpec{
			Type:  "peer",
			MspID: "Org1MSP",
			Enrollment: &fabricxv1alpha1.IdentityEnrollment{
				CARef: fabricxv1alpha1.IdentityCARef{
					Name:      "test-ca",
					Namespace: namespace,
				},
				EnrollID: adminUser,
				EnrollSecretRef: fabricxv1alpha1.SecretKeyNSSelector{
					Name:      "test-enroll-secret",
					Key:       "password",
					Namespace: namespace,
				},
				EnrollTLS: true, // Enable TLS enrollment
			},
			Output: fabricxv1alpha1.IdentityOutput{
				SecretPrefix: "test-identity-dual",
				Namespace:    namespace,
			},
		},
	}
	err := k8sClient.Create(ctx, identity)
	require.NoError(t, err, "Failed to create Identity resource")

	// Wait for identity to be ready
	err = waitForIdentityReady(ctx, k8sClient, types.NamespacedName{
		Name:      "test-identity-dual",
		Namespace: namespace,
	}, 60*time.Second)
	require.NoError(t, err, "Identity should become ready")

	// Verify TLS secrets were created
	tlsCertSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Name:      "test-identity-dual-tls-cert",
		Namespace: namespace,
	}, tlsCertSecret)
	require.NoError(t, err, "TLS cert secret should exist")
	assert.NotEmpty(t, tlsCertSecret.Data["cert.pem"], "TLS cert should not be empty")

	// Verify combined TLS secret
	combinedTLSSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Name:      "test-identity-dual-tls-combined",
		Namespace: namespace,
	}, combinedTLSSecret)
	require.NoError(t, err, "Combined TLS secret should exist")
	assert.NotEmpty(t, combinedTLSSecret.Data["cert.pem"], "Combined TLS cert should not be empty")
	assert.NotEmpty(t, combinedTLSSecret.Data["key.pem"], "Combined TLS key should not be empty")
	assert.NotEmpty(t, combinedTLSSecret.Data["ca.pem"], "Combined TLS CA cert should not be empty")

	t.Logf("✅ Dual (sign + TLS) enrollment completed successfully")
}

// testIdemixEnrollment tests idemix enrollment
func testIdemixEnrollment(t *testing.T, ctx context.Context, k8sClient client.Client, caURL, tlsCert string) {
	t.Skip("Idemix enrollment requires CA with idemix support - implement when needed")
	// TODO: Start CA with --idemix.curve gurvy.Bn254
	// TODO: Create Identity with Idemix enrollment enabled
	// TODO: Verify idemix credential secret is created
}

// testSecretCleanupOnDeletion tests that secrets are deleted when identity is deleted
func testSecretCleanupOnDeletion(t *testing.T, ctx context.Context, k8sClient client.Client, caURL, tlsCert string) {
	namespace := "default"

	// Create Identity resource
	identity := &fabricxv1alpha1.Identity{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-identity-cleanup",
			Namespace: namespace,
		},
		Spec: fabricxv1alpha1.IdentitySpec{
			Type:  "user",
			MspID: "Org1MSP",
			Enrollment: &fabricxv1alpha1.IdentityEnrollment{
				CARef: fabricxv1alpha1.IdentityCARef{
					Name:      "test-ca",
					Namespace: namespace,
				},
				EnrollID: adminUser,
				EnrollSecretRef: fabricxv1alpha1.SecretKeyNSSelector{
					Name:      "test-enroll-secret",
					Key:       "password",
					Namespace: namespace,
				},
				EnrollTLS: false,
			},
			Output: fabricxv1alpha1.IdentityOutput{
				SecretPrefix: "test-identity-cleanup",
				Namespace:    namespace,
			},
		},
	}
	err := k8sClient.Create(ctx, identity)
	require.NoError(t, err, "Failed to create Identity resource")

	// Wait for identity to be ready
	err = waitForIdentityReady(ctx, k8sClient, types.NamespacedName{
		Name:      "test-identity-cleanup",
		Namespace: namespace,
	}, 60*time.Second)
	require.NoError(t, err, "Identity should become ready")

	// Verify secret exists
	signCertSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Name:      "test-identity-cleanup-sign-cert",
		Namespace: namespace,
	}, signCertSecret)
	require.NoError(t, err, "Sign cert secret should exist")

	// Delete identity
	err = k8sClient.Delete(ctx, identity)
	require.NoError(t, err, "Failed to delete Identity resource")

	// Wait for identity to be deleted
	err = waitForIdentityDeleted(ctx, k8sClient, types.NamespacedName{
		Name:      "test-identity-cleanup",
		Namespace: namespace,
	}, 30*time.Second)
	require.NoError(t, err, "Identity should be deleted")

	// Verify secrets are deleted (due to ownerReferences)
	time.Sleep(5 * time.Second) // Give garbage collector time
	signCertSecret = &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{
		Name:      "test-identity-cleanup-sign-cert",
		Namespace: namespace,
	}, signCertSecret)
	assert.Error(t, err, "Secret should be deleted")

	t.Logf("✅ Secret cleanup on deletion verified")
}

// setupTestEnvironment sets up the Kubernetes test environment and starts the controller
func setupTestEnvironment(t *testing.T, ctx context.Context) (*envtest.Environment, client.Client, func()) {
	t.Log("Setting up test environment...")

	// Start test environment
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err, "Failed to start test environment")
	require.NotNil(t, cfg, "Test environment config should not be nil")

	// Setup scheme
	scheme := runtime.NewScheme()
	err = fabricxv1alpha1.AddToScheme(scheme)
	require.NoError(t, err, "Failed to add fabricx scheme")
	err = corev1.AddToScheme(scheme)
	require.NoError(t, err, "Failed to add core scheme")

	// Create client
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "Failed to create k8s client")

	// Start controller manager
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	require.NoError(t, err, "Failed to create manager")

	// Setup Identity controller
	identityReconciler := &controller.IdentityReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	err = identityReconciler.SetupWithManager(mgr)
	require.NoError(t, err, "Failed to setup Identity controller")

	// Start manager in background
	mgrCtx, mgrCancel := context.WithCancel(ctx)
	go func() {
		err := mgr.Start(mgrCtx)
		if err != nil {
			t.Logf("Manager stopped with error: %v", err)
		}
	}()

	// Wait for manager to be ready
	time.Sleep(2 * time.Second)

	stopController := func() {
		t.Log("Stopping controller...")
		mgrCancel()
	}

	t.Log("✅ Test environment ready")
	return testEnv, k8sClient, stopController
}

// startFabricCAContainer starts a Fabric CA container for testing
func startFabricCAContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string, string, func()) {
	// Create temporary directory for CA files
	tmpDir, err := os.MkdirTemp("", "fabric-ca-e2e-*")
	require.NoError(t, err)

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
	require.NoError(t, err)

	// Get mapped port
	mappedPort, err := container.MappedPort(ctx, "7054")
	require.NoError(t, err)

	// Get host
	host, err := container.Host(ctx)
	require.NoError(t, err)

	caURL := fmt.Sprintf("https://%s:%s", host, mappedPort.Port())

	// Wait for CA to be fully ready
	time.Sleep(5 * time.Second)

	// Get TLS certificate from container
	tlsCertPath, err := getTLSCertFromContainer(ctx, container, tmpDir)
	require.NoError(t, err)

	cleanup := func() {
		container.Terminate(ctx)
		os.RemoveAll(tmpDir)
	}

	return container, caURL, tlsCertPath, cleanup
}

// getTLSCertFromContainer retrieves the TLS certificate from the container
func getTLSCertFromContainer(ctx context.Context, container testcontainers.Container, tmpDir string) (string, error) {
	time.Sleep(2 * time.Second)

	exitCode, reader, err := container.Exec(ctx, []string{"cat", "/etc/hyperledger/fabric-ca-server/tls-cert.pem"})
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", fmt.Errorf("cat command returned non-zero exit code: %d", exitCode)
	}

	tlsCertBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	// Trim to PEM start
	tlsCert := []byte(string(tlsCertBytes))
	pemStart := []byte("-----BEGIN CERTIFICATE-----")
	for i := range tlsCert {
		if i+len(pemStart) <= len(tlsCert) && string(tlsCert[i:i+len(pemStart)]) == string(pemStart) {
			tlsCert = tlsCert[i:]
			break
		}
	}

	tlsCertPath := filepath.Join(tmpDir, "tls-cert.pem")
	if err := os.WriteFile(tlsCertPath, tlsCert, 0644); err != nil {
		return "", err
	}

	return tlsCertPath, nil
}

// waitForIdentityReady waits for identity to reach Ready status
func waitForIdentityReady(ctx context.Context, k8sClient client.Client, name types.NamespacedName, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for identity to be ready")
		case <-ticker.C:
			identity := &fabricxv1alpha1.Identity{}
			err := k8sClient.Get(ctx, name, identity)
			if err != nil {
				continue
			}
			if identity.Status.Status == "READY" {
				return nil
			}
			if identity.Status.Status == "FAILED" {
				return fmt.Errorf("identity failed: %s", identity.Status.Message)
			}
		}
	}
}

// waitForIdentityDeleted waits for identity to be deleted
func waitForIdentityDeleted(ctx context.Context, k8sClient client.Client, name types.NamespacedName, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for identity to be deleted")
		case <-ticker.C:
			identity := &fabricxv1alpha1.Identity{}
			err := k8sClient.Get(ctx, name, identity)
			if err != nil && client.IgnoreNotFound(err) == nil {
				return nil // Successfully deleted
			}
		}
	}
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[len(s)-maxLen:]
}
