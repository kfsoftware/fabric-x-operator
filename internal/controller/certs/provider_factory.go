package certs

import (
	"fmt"
)

// ProviderFactory creates certificate providers based on configuration
type ProviderFactory struct {
	// Registry of available providers
	providers map[string]func() CertificateProvider
}

// NewProviderFactory creates a new provider factory with default providers registered
func NewProviderFactory() *ProviderFactory {
	factory := &ProviderFactory{
		providers: make(map[string]func() CertificateProvider),
	}

	// Register built-in providers
	factory.Register("fabric-ca", func() CertificateProvider {
		return NewFabricCAProvider()
	})
	factory.Register("manual", func() CertificateProvider {
		return NewManualProvider()
	})
	factory.Register("vault", func() CertificateProvider {
		return NewVaultProvider()
	})

	return factory
}

// Register adds a new provider to the factory
// This allows custom providers to be registered at runtime
func (f *ProviderFactory) Register(providerType string, constructor func() CertificateProvider) {
	f.providers[providerType] = constructor
}

// GetProvider returns a certificate provider for the given type
func (f *ProviderFactory) GetProvider(providerType string) (CertificateProvider, error) {
	constructor, ok := f.providers[providerType]
	if !ok {
		return nil, fmt.Errorf("unknown certificate provider type: %q (available: fabric-ca, manual, vault)", providerType)
	}
	return constructor(), nil
}

// GetProviderFromConfig determines the provider type from configuration and returns the appropriate provider
func (f *ProviderFactory) GetProviderFromConfig(providerConfig *ProviderConfig) (CertificateProvider, interface{}, error) {
	if providerConfig == nil {
		return nil, nil, fmt.Errorf("provider configuration is required")
	}

	provider, err := f.GetProvider(providerConfig.Type)
	if err != nil {
		return nil, nil, err
	}

	// Extract the provider-specific config
	var config interface{}
	switch providerConfig.Type {
	case "fabric-ca":
		if providerConfig.FabricCA == nil {
			return nil, nil, fmt.Errorf("fabricCA configuration is required when type is 'fabric-ca'")
		}
		config = providerConfig.FabricCA
	case "manual":
		if providerConfig.Manual == nil {
			return nil, nil, fmt.Errorf("manual configuration is required when type is 'manual'")
		}
		config = providerConfig.Manual
	case "vault":
		if providerConfig.Vault == nil {
			return nil, nil, fmt.Errorf("vault configuration is required when type is 'vault'")
		}
		config = providerConfig.Vault
	default:
		return nil, nil, fmt.Errorf("unsupported provider type: %q", providerConfig.Type)
	}

	return provider, config, nil
}

// Default factory instance that can be used across all controllers
var DefaultProviderFactory = NewProviderFactory()
