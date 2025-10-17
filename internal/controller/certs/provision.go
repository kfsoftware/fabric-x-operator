package certs

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProvisionCertificatesOptions contains options for certificate provisioning
type ProvisionCertificatesOptions struct {
	// Kubernetes client for accessing secrets
	K8sClient client.Client

	// MSP ID for the organization
	MSPID string

	// Component name (for cert CN and secret names)
	ComponentName string

	// Sign certificate provider configuration
	SignProvider *ProviderConfig

	// TLS certificate provider configuration
	TLSProvider *ProviderConfig

	// DNS names for TLS certificate SANs
	DNSNames []string

	// IP addresses for TLS certificate SANs
	IPAddresses []string

	// Custom provider factory (optional, uses DefaultProviderFactory if nil)
	ProviderFactory *ProviderFactory
}

// ProvisionCertificatesResult contains the results of certificate provisioning
type ProvisionCertificatesResult struct {
	// Sign certificate data (nil if not requested)
	SignCertificate *CertificateData

	// TLS certificate data (nil if not requested)
	TLSCertificate *CertificateData
}

// ProvisionCertificates is the main entry point for certificate provisioning
// This function can be used by any controller (Endorser, Committer, OrdererGroup, etc.)
// to provision certificates using any supported provider
func ProvisionCertificates(ctx context.Context, opts ProvisionCertificatesOptions) (*ProvisionCertificatesResult, error) {
	result := &ProvisionCertificatesResult{}

	// Use default factory if not provided
	factory := opts.ProviderFactory
	if factory == nil {
		factory = DefaultProviderFactory
	}

	// Provision sign certificate if requested
	if opts.SignProvider != nil {
		provider, config, err := factory.GetProviderFromConfig(opts.SignProvider)
		if err != nil {
			return nil, fmt.Errorf("failed to get sign certificate provider: %w", err)
		}

		signCertData, err := provider.ProvisionSignCertificate(ctx, SignCertificateRequest{
			K8sClient:     opts.K8sClient,
			MSPID:         opts.MSPID,
			ComponentName: opts.ComponentName,
			Config:        config,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to provision sign certificate with provider %q: %w", provider.Name(), err)
		}

		result.SignCertificate = signCertData
	}

	// Provision TLS certificate if requested
	if opts.TLSProvider != nil {
		provider, config, err := factory.GetProviderFromConfig(opts.TLSProvider)
		if err != nil {
			return nil, fmt.Errorf("failed to get TLS certificate provider: %w", err)
		}

		tlsCertData, err := provider.ProvisionTLSCertificate(ctx, TLSCertificateRequest{
			K8sClient:     opts.K8sClient,
			MSPID:         opts.MSPID,
			ComponentName: opts.ComponentName,
			DNSNames:      opts.DNSNames,
			IPAddresses:   opts.IPAddresses,
			Config:        config,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to provision TLS certificate with provider %q: %w", provider.Name(), err)
		}

		result.TLSCertificate = tlsCertData
	}

	return result, nil
}

// Example usage in a controller:
//
// func (r *EndorserReconciler) reconcileCertificates(ctx context.Context, endorser *fabricxv1alpha1.Endorser) error {
//     // Build provider configs from CRD spec
//     signProvider := &certs.ProviderConfig{
//         Type: "fabric-ca",
//         FabricCA: &certs.FabricCAConfig{
//             CAHost:       endorser.Spec.Enrollment.Sign.CA.CAHost,
//             CAPort:       endorser.Spec.Enrollment.Sign.CA.CAPort,
//             CAName:       endorser.Spec.Enrollment.Sign.CA.CAName,
//             EnrollID:     endorser.Spec.Enrollment.Sign.CA.EnrollID,
//             EnrollSecret: endorser.Spec.Enrollment.Sign.CA.EnrollSecret,
//             CATLS:        endorser.Spec.Enrollment.Sign.CA.CATLS,
//         },
//     }
//
//     tlsProvider := &certs.ProviderConfig{
//         Type: "fabric-ca",
//         FabricCA: &certs.FabricCAConfig{
//             CAHost:       endorser.Spec.Enrollment.TLS.CA.CAHost,
//             CAPort:       endorser.Spec.Enrollment.TLS.CA.CAPort,
//             CAName:       endorser.Spec.Enrollment.TLS.CA.CAName,
//             EnrollID:     endorser.Spec.Enrollment.TLS.CA.EnrollID,
//             EnrollSecret: endorser.Spec.Enrollment.TLS.CA.EnrollSecret,
//             CATLS:        endorser.Spec.Enrollment.TLS.CA.CATLS,
//         },
//     }
//
//     // Provision certificates
//     result, err := certs.ProvisionCertificates(ctx, certs.ProvisionCertificatesOptions{
//         K8sClient:     r.Client,
//         MSPID:         endorser.Spec.MSPID,
//         ComponentName: endorser.Name,
//         SignProvider:  signProvider,
//         TLSProvider:   tlsProvider,
//         DNSNames:      endorser.Spec.SANS.DNSNames,
//         IPAddresses:   endorser.Spec.SANS.IPAddresses,
//     })
//     if err != nil {
//         return fmt.Errorf("failed to provision certificates: %w", err)
//     }
//
//     // Create secrets from result
//     if result.SignCertificate != nil {
//         // Create sign-cert secret
//     }
//     if result.TLSCertificate != nil {
//         // Create tls-cert secret
//     }
//
//     return nil
// }
