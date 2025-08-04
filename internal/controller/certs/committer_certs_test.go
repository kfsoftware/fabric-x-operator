package certs

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

func TestCommitterCertService_CleanupComponentCertificates(t *testing.T) {
	// Create a fake client
	client := fake.NewClientBuilder().Build()

	// Create a scheme
	scheme := runtime.NewScheme()

	// Create the certificate service
	certService := NewCommitterCertService(client, scheme)

	// Create a test committer
	committer := &fabricxv1alpha1.Committer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-committer",
			Namespace: "test-namespace",
		},
		Spec: fabricxv1alpha1.CommitterSpec{
			MSPID: "TestMSP",
			Enrollment: &fabricxv1alpha1.EnrollmentConfig{
				Sign: &fabricxv1alpha1.CertificateConfig{
					EnrollID:     "test-sign-id",
					EnrollSecret: "test-sign-secret",
				},
				TLS: &fabricxv1alpha1.CertificateConfig{
					EnrollID:     "test-tls-id",
					EnrollSecret: "test-tls-secret",
				},
			},
		},
	}

	// Test cleanup - should not fail since it's now a no-op
	// (secrets are automatically cleaned up via owner references)
	err := certService.CleanupComponentCertificates(context.Background(), committer, "coordinator")
	if err != nil {
		t.Errorf("Cleanup should not fail: %v", err)
	}

	// Note: In a real scenario, secrets would be automatically deleted by Kubernetes
	// when the parent resource is deleted due to owner references.
	// This test just verifies that the cleanup method doesn't error.
}

func TestCommitterCertService_GetCertificateSecretName(t *testing.T) {
	// Create a scheme
	scheme := runtime.NewScheme()

	// Create the certificate service
	certService := NewCommitterCertService(nil, scheme)

	// Test secret name generation
	secretName := certService.GetCertificateSecretName("test-committer", "coordinator", 0, "sign")
	expectedName := "test-committer-coordinator-sign-cert"
	if secretName != expectedName {
		t.Errorf("Expected secret name %s, got %s", expectedName, secretName)
	}

	secretName = certService.GetCertificateSecretName("test-committer", "sidecar", 1, "tls")
	expectedName = "test-committer-sidecar-tls-cert"
	if secretName != expectedName {
		t.Errorf("Expected secret name %s, got %s", expectedName, secretName)
	}
}
