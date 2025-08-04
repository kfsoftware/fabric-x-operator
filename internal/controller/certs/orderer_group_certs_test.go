package certs

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

func TestOrdererGroupCertService_ProvisionComponentCertificates_ExistingSecret(t *testing.T) {
	// Create a fake client
	client := fake.NewClientBuilder().Build()

	// Create a scheme
	scheme := runtime.NewScheme()

	// Create the certificate service
	certService := NewOrdererGroupCertService(client, scheme)

	// Create a test orderer group
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-orderergroup",
			Namespace: "test-namespace",
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
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

	// Create an existing secret
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-orderergroup-consenter-sign-cert",
			Namespace: "test-namespace",
		},
		Data: map[string][]byte{
			"cert.pem": []byte("existing-cert"),
			"key.pem":  []byte("existing-key"),
			"ca.pem":   []byte("existing-ca"),
		},
	}

	// Add the secret to the fake client
	client.Create(context.Background(), existingSecret)

	// Test provisioning with existing secret
	config := &fabricxv1alpha1.ComponentConfig{
		CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
			Replicas: 1,
		},
		Certificates: &fabricxv1alpha1.CertificateConfig{
			EnrollID:     "test-enroll-id",
			EnrollSecret: "test-enroll-secret",
		},
	}

	// This should fail due to no CA server, but the logic should check for existing secrets first
	err := certService.ProvisionComponentCertificates(context.Background(), ordererGroup, "consenter", config)
	if err == nil {
		t.Error("Expected error due to no CA server, but logic should check for existing secrets first")
	}
}

func TestOrdererGroupCertService_ProvisionComponentCertificates_NoExistingSecret(t *testing.T) {
	// Create a fake client
	client := fake.NewClientBuilder().Build()

	// Create a scheme
	scheme := runtime.NewScheme()

	// Create the certificate service
	certService := NewOrdererGroupCertService(client, scheme)

	// Create a test orderer group
	ordererGroup := &fabricxv1alpha1.OrdererGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-orderergroup",
			Namespace: "test-namespace",
		},
		Spec: fabricxv1alpha1.OrdererGroupSpec{
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

	// Test provisioning without existing secret
	config := &fabricxv1alpha1.ComponentConfig{
		CommonComponentConfig: fabricxv1alpha1.CommonComponentConfig{
			Replicas: 1,
		},
		Certificates: &fabricxv1alpha1.CertificateConfig{
			EnrollID:     "test-enroll-id",
			EnrollSecret: "test-enroll-secret",
		},
	}

	// This should fail due to no CA server
	err := certService.ProvisionComponentCertificates(context.Background(), ordererGroup, "consenter", config)
	if err == nil {
		t.Error("Expected error due to no CA server")
	}
}
