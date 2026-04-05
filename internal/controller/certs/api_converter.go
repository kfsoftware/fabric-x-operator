package certs

import (
	"fmt"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

// ConvertCertificateConfigToProviderConfig converts API CertificateConfig to certs.ProviderConfig
// This function handles backward compatibility with the old CA field
func ConvertCertificateConfigToProviderConfig(apiConfig *fabricxv1alpha1.CertificateConfig) (*ProviderConfig, error) {
	if apiConfig == nil {
		return nil, nil
	}

	// Determine provider type
	providerType := apiConfig.ProviderType

	// Backward compatibility: if ProviderType not specified, infer from configuration
	if providerType == "" {
		if apiConfig.CA != nil {
			providerType = "fabric-ca"
		} else if apiConfig.Provider != nil {
			// Infer from Provider field
			if apiConfig.Provider.FabricCA != nil {
				providerType = "fabric-ca"
			} else if apiConfig.Provider.Manual != nil {
				providerType = "manual"
			} else if apiConfig.Provider.Vault != nil {
				providerType = "vault"
			}
		} else {
			// Default to manual if no provider configuration
			providerType = "manual"
		}
	}

	providerConfig := &ProviderConfig{
		Type: providerType,
	}

	switch providerType {
	case "fabric-ca":
		// Try new Provider field first, fall back to old CA field
		if apiConfig.Provider != nil && apiConfig.Provider.FabricCA != nil {
			providerConfig.FabricCA = convertFabricCAProviderConfig(apiConfig.Provider.FabricCA)
		} else if apiConfig.CA != nil {
			// Backward compatibility with old CA field
			providerConfig.FabricCA = convertLegacyCAConfig(apiConfig.CA)
		} else {
			return nil, fmt.Errorf("fabric-ca provider requires CA configuration")
		}

	case "manual":
		if apiConfig.Provider != nil && apiConfig.Provider.Manual != nil {
			providerConfig.Manual = convertManualProviderConfig(apiConfig.Provider.Manual)
		} else {
			return nil, fmt.Errorf("manual provider requires manual configuration")
		}

	case "vault":
		if apiConfig.Provider != nil && apiConfig.Provider.Vault != nil {
			providerConfig.Vault = convertVaultProviderConfig(apiConfig.Provider.Vault)
		} else {
			return nil, fmt.Errorf("vault provider requires Vault configuration")
		}

	default:
		return nil, fmt.Errorf("unsupported provider type: %q", providerType)
	}

	return providerConfig, nil
}

// convertFabricCAProviderConfig converts API FabricCAProviderConfig to certs.FabricCAConfig
func convertFabricCAProviderConfig(api *fabricxv1alpha1.FabricCAProviderConfig) *FabricCAConfig {
	if api == nil {
		return nil
	}

	config := &FabricCAConfig{
		CAHost:       api.CAHost,
		CAPort:       int(api.CAPort),
		CAName:       api.CAName,
		EnrollID:     api.EnrollID,
		EnrollSecret: api.EnrollSecret,
	}

	if api.CATLS != nil {
		config.CATLS = &CATLSConfig{
			CACert: api.CATLS.CACert,
		}
		if api.CATLS.SecretRef != nil {
			config.CATLS.SecretRef = &SecretRef{
				Name:      api.CATLS.SecretRef.Name,
				Key:       api.CATLS.SecretRef.Key,
				Namespace: api.CATLS.SecretRef.Namespace,
			}
		}
	}

	return config
}

// convertLegacyCAConfig converts legacy API CACertificateConfig to certs.FabricCAConfig
// This provides backward compatibility with the old CA field
func convertLegacyCAConfig(api *fabricxv1alpha1.CACertificateConfig) *FabricCAConfig {
	if api == nil {
		return nil
	}

	config := &FabricCAConfig{
		CAHost:       api.CAHost,
		CAPort:       int(api.CAPort),
		CAName:       api.CAName,
		EnrollID:     api.EnrollID,
		EnrollSecret: api.EnrollSecret,
	}

	if api.CATLS != nil {
		config.CATLS = &CATLSConfig{
			CACert: api.CATLS.CACert,
		}
		if api.CATLS.SecretRef != nil {
			config.CATLS.SecretRef = &SecretRef{
				Name:      api.CATLS.SecretRef.Name,
				Key:       api.CATLS.SecretRef.Key,
				Namespace: api.CATLS.SecretRef.Namespace,
			}
		}
	}

	return config
}

// convertManualProviderConfig converts API ManualProviderConfig to certs.ManualConfig
func convertManualProviderConfig(api *fabricxv1alpha1.ManualProviderConfig) *ManualConfig {
	if api == nil {
		return nil
	}

	config := &ManualConfig{
		CertKey: api.CertKey,
		KeyKey:  api.KeyKey,
		CAKey:   api.CAKey,
	}

	if api.SecretRef != nil {
		config.SecretRef = &SecretRef{
			Name:      api.SecretRef.Name,
			Key:       api.SecretRef.Key,
			Namespace: api.SecretRef.Namespace,
		}
	}

	return config
}

// convertVaultProviderConfig converts API VaultProviderConfig to certs.VaultConfig
func convertVaultProviderConfig(api *fabricxv1alpha1.VaultProviderConfig) *VaultConfig {
	if api == nil {
		return nil
	}

	config := &VaultConfig{
		Address:        api.Address,
		PKIPath:        api.PKIPath,
		Role:           api.Role,
		AuthMethod:     api.AuthMethod,
		ServiceAccount: api.ServiceAccount,
		Namespace:      api.Namespace,
		TTL:            api.TTL,
	}

	if api.TokenSecretRef != nil {
		config.TokenSecretRef = &SecretRef{
			Name:      api.TokenSecretRef.Name,
			Key:       api.TokenSecretRef.Key,
			Namespace: api.TokenSecretRef.Namespace,
		}
	}

	return config
}
