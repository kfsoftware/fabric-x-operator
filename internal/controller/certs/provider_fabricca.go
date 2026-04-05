package certs

import (
	"context"
	"fmt"
)

// FabricCAProvider implements the CertificateProvider interface for Fabric CA
type FabricCAProvider struct{}

// NewFabricCAProvider creates a new Fabric CA certificate provider
func NewFabricCAProvider() *FabricCAProvider {
	return &FabricCAProvider{}
}

// Name returns the provider name
func (p *FabricCAProvider) Name() string {
	return "fabric-ca"
}

// ProvisionSignCertificate provisions a signing certificate from Fabric CA
func (p *FabricCAProvider) ProvisionSignCertificate(ctx context.Context, req SignCertificateRequest) (*CertificateData, error) {
	// Extract Fabric CA config from the request
	fabricCAConfig, ok := req.Config.(*FabricCAConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type for Fabric CA provider, expected *FabricCAConfig")
	}

	// Build certificate configuration
	certConfig := &CertificateConfig{
		MSPID: req.MSPID,
		CA: &CACertificateConfig{
			CAHost:       fabricCAConfig.CAHost,
			CAPort:       int32(fabricCAConfig.CAPort),
			CAName:       fabricCAConfig.CAName,
			EnrollID:     fabricCAConfig.EnrollID,
			EnrollSecret: fabricCAConfig.EnrollSecret,
			CATLS:        fabricCAConfig.CATLS,
		},
	}

	// Build request
	certRequest := OrdererGroupCertificateRequest{
		ComponentName: req.ComponentName,
		ComponentType: req.ComponentName,
		CertConfig:    certConfig,
	}

	// Use the existing enrollment code
	certData, err := provisionComponentCertificateWithClient(
		ctx,
		req.K8sClient,
		certRequest,
		"sign",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to provision sign certificate from Fabric CA: %w", err)
	}

	if certData == nil {
		return nil, nil
	}

	return &CertificateData{
		Certificate:   certData.Cert,
		PrivateKey:    certData.Key,
		CACertificate: certData.CACert,
		Type:          "sign",
	}, nil
}

// ProvisionTLSCertificate provisions a TLS certificate from Fabric CA
func (p *FabricCAProvider) ProvisionTLSCertificate(ctx context.Context, req TLSCertificateRequest) (*CertificateData, error) {
	// Extract Fabric CA config from the request
	fabricCAConfig, ok := req.Config.(*FabricCAConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type for Fabric CA provider, expected *FabricCAConfig")
	}

	// Build certificate configuration
	certConfig := &CertificateConfig{
		MSPID: req.MSPID,
		CA: &CACertificateConfig{
			CAHost:       fabricCAConfig.CAHost,
			CAPort:       int32(fabricCAConfig.CAPort),
			CAName:       fabricCAConfig.CAName,
			EnrollID:     fabricCAConfig.EnrollID,
			EnrollSecret: fabricCAConfig.EnrollSecret,
			CATLS:        fabricCAConfig.CATLS,
		},
	}

	// Add SANS if provided
	if len(req.DNSNames) > 0 || len(req.IPAddresses) > 0 {
		certConfig.SANS = &SANSConfig{
			DNSNames:    req.DNSNames,
			IPAddresses: req.IPAddresses,
		}
	}

	// Build request
	certRequest := OrdererGroupCertificateRequest{
		ComponentName: req.ComponentName,
		ComponentType: req.ComponentName,
		CertConfig:    certConfig,
	}

	// Use the existing enrollment code
	certData, err := provisionComponentCertificateWithClient(
		ctx,
		req.K8sClient,
		certRequest,
		"tls",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to provision TLS certificate from Fabric CA: %w", err)
	}

	if certData == nil {
		return nil, nil
	}

	return &CertificateData{
		Certificate:   certData.Cert,
		PrivateKey:    certData.Key,
		CACertificate: certData.CACert,
		Type:          "tls",
	}, nil
}
