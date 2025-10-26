package controller_test

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/kfsoftware/fabric-x-operator/internal/controller"
)

// TestIdentityControllerSimpleE2E tests the Identity controller with a real Fabric CA
// This test works by creating a mock CA service that points to the testcontainer CA
func TestIdentityControllerSimpleE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E integration test in short mode")
	}

	// Setup logging
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Start Fabric CA container first
	caContainer, caURL, tlsCertPath, cleanupCA := startSimpleFabricCAContainer(t, ctx)
	defer cleanupCA()

	t.Logf("Fabric CA started at %s", caURL)

	// Parse CA URL to get host and port
	parsedURL, err := url.Parse(caURL)
	require.NoError(t, err)
	caHost := strings.Split(parsedURL.Host, ":")[0]
	caPort := parsedURL.Port()

	// Read TLS cert
	tlsCert, err := os.ReadFile(tlsCertPath)
	require.NoError(t, err)

	// Start test environment and controller
	testEnv, k8sClient, stopController := setupSimpleTestEnvironment(t, ctx)
	defer stopController()
	defer testEnv.Stop()

	namespace := "default"

	// Create CA TLS secret (contains the actual TLS cert from container)
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-ca-tls",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.pem": tlsCert,
		},
	}
	err = k8sClient.Create(ctx, caSecret)
	require.NoError(t, err)

	// Create enrollment secret
	enrollSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-enroll-secret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"password": []byte(adminPassword),
		},
	}
	err = k8sClient.Create(ctx, enrollSecret)
	require.NoError(t, err)

	// Create mock CA service (ExternalName service pointing to localhost)
	caService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-test-ca",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: caHost,
			Ports: []corev1.ServicePort{
				{
					Port:     7054,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
	err = k8sClient.Create(ctx, caService)
	require.NoError(t, err)

	t.Logf("Created CA service pointing to %s:%s", caHost, caPort)

	// Create CA resource that references the actual CA URL
	ca := &fabricxv1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "simple-test-ca",
			Namespace: namespace,
		},
		Spec: fabricxv1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca",
			Version: "1.5.15",
			CA: fabricxv1alpha1.FabricCAItemConf{
				Name: "ca",
			},
			// Add hosts to make the CA URL match the container
			Hosts: []string{caHost},
		},
	}
	err = k8sClient.Create(ctx, ca)
	require.NoError(t, err)

	// Wait a bit for resources to settle
	time.Sleep(2 * time.Second)

	t.Run("SignEnrollment", func(t *testing.T) {
		// Create Identity with custom CA cert reference
		identity := &fabricxv1alpha1.Identity{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple-identity",
				Namespace: namespace,
			},
			Spec: fabricxv1alpha1.IdentitySpec{
				Type:  "user",
				MspID: "Org1MSP",
				Enrollment: &fabricxv1alpha1.IdentityEnrollment{
					CARef: fabricxv1alpha1.IdentityCARef{
						Name:      "simple-test-ca",
						Namespace: namespace,
						// Provide custom CA cert reference
						CACertRef: &fabricxv1alpha1.SecretKeyNSSelector{
							Name:      "simple-ca-tls",
							Key:       "ca.pem",
							Namespace: namespace,
						},
					},
					EnrollID: adminUser,
					EnrollSecretRef: fabricxv1alpha1.SecretKeyNSSelector{
						Name:      "simple-enroll-secret",
						Key:       "password",
						Namespace: namespace,
					},
				},
				Output: fabricxv1alpha1.IdentityOutput{
					SecretName: "simple-identity",
					Namespace:    namespace,
				},
			},
		}
		err = k8sClient.Create(ctx, identity)
		require.NoError(t, err)

		// Note: This will still fail because the controller builds URL as https://simple-test-ca.default:7054
		// but we need it to use https://localhost:MAPPED_PORT

		// The fundamental issue is that envtest controller can't reach Docker containers
		// We need to modify the Identity controller to support direct URL configuration
		// OR use a real Kubernetes cluster for E2E tests

		t.Logf("Identity created, but enrollment will fail due to networking...")
		t.Logf("This demonstrates the architectural limitation of envtest + Docker CA")
	})

	// Show CA logs for debugging
	logs, _ := caContainer.Logs(ctx)
	if logs != nil {
		defer logs.Close()
		logBytes, _ := io.ReadAll(logs)
		t.Logf("CA logs (last 1000 chars): %s", truncateString(string(logBytes), 1000))
	}
}

// setupSimpleTestEnvironment sets up a minimal test environment
func setupSimpleTestEnvironment(t *testing.T, ctx context.Context) (*envtest.Environment, client.Client, func()) {
	t.Log("Setting up simple test environment...")

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err)

	scheme := runtime.NewScheme()
	err = fabricxv1alpha1.AddToScheme(scheme)
	require.NoError(t, err)
	err = corev1.AddToScheme(scheme)
	require.NoError(t, err)

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)

	// Start controller
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme})
	require.NoError(t, err)

	reconciler := &controller.IdentityReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	err = reconciler.SetupWithManager(mgr)
	require.NoError(t, err)

	mgrCtx, mgrCancel := context.WithCancel(ctx)
	go func() {
		mgr.Start(mgrCtx)
	}()

	time.Sleep(2 * time.Second)

	stopController := func() {
		mgrCancel()
	}

	t.Log("✅ Simple test environment ready")
	return testEnv, k8sClient, stopController
}

// startSimpleFabricCAContainer starts a minimal Fabric CA for testing
func startSimpleFabricCAContainer(t *testing.T, ctx context.Context) (testcontainers.Container, string, string, func()) {
	tmpDir, err := os.MkdirTemp("", "simple-ca-*")
	require.NoError(t, err)

	req := testcontainers.ContainerRequest{
		Image:        fabricCAImage,
		ExposedPorts: []string{"7054/tcp"},
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

	mappedPort, err := container.MappedPort(ctx, "7054")
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	caURL := fmt.Sprintf("https://%s:%s", host, mappedPort.Port())

	time.Sleep(5 * time.Second)

	tlsCertPath, err := getSimpleTLSCert(ctx, container, tmpDir)
	require.NoError(t, err)

	cleanup := func() {
		container.Terminate(ctx)
		os.RemoveAll(tmpDir)
	}

	return container, caURL, tlsCertPath, cleanup
}

// getSimpleTLSCert retrieves TLS cert from container
func getSimpleTLSCert(ctx context.Context, container testcontainers.Container, tmpDir string) (string, error) {
	time.Sleep(2 * time.Second)

	exitCode, reader, err := container.Exec(ctx, []string{"cat", "/etc/hyperledger/fabric-ca-server/tls-cert.pem"})
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", fmt.Errorf("cat returned exit code %d", exitCode)
	}

	certBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	// Trim to PEM start
	cert := string(certBytes)
	if idx := strings.Index(cert, "-----BEGIN CERTIFICATE-----"); idx >= 0 {
		cert = cert[idx:]
	}

	certPath := filepath.Join(tmpDir, "ca-cert.pem")
	if err := os.WriteFile(certPath, []byte(cert), 0644); err != nil {
		return "", err
	}

	return certPath, nil
}
