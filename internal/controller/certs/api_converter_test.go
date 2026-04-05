package certs

import (
	"testing"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

func TestConvertCertificateConfigToProviderConfig(t *testing.T) {
	t.Run("Fabric CA with legacy CA field", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.CertificateConfig{
			CA: &fabricxv1alpha1.CACertificateConfig{
				CAHost:       "ca.example.com",
				CAPort:       7054,
				CAName:       "ca-org1",
				EnrollID:     "admin",
				EnrollSecret: "adminpw",
				CATLS: &fabricxv1alpha1.CATLSConfig{
					CACert: "base64cert",
				},
			},
		}

		providerConfig, err := ConvertCertificateConfigToProviderConfig(apiConfig)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if providerConfig.Type != "fabric-ca" {
			t.Errorf("Expected type 'fabric-ca', got '%s'", providerConfig.Type)
		}

		if providerConfig.FabricCA == nil {
			t.Fatal("FabricCA config is nil")
		}

		if providerConfig.FabricCA.CAHost != "ca.example.com" {
			t.Errorf("Expected CAHost 'ca.example.com', got '%s'", providerConfig.FabricCA.CAHost)
		}

		if providerConfig.FabricCA.CAPort != 7054 {
			t.Errorf("Expected CAPort 7054, got %d", providerConfig.FabricCA.CAPort)
		}
	})

	t.Run("Fabric CA with new provider field", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.CertificateConfig{
			ProviderType: "fabric-ca",
			Provider: &fabricxv1alpha1.CertificateProviderConfig{
				FabricCA: &fabricxv1alpha1.FabricCAProviderConfig{
					CAHost:       "ca.new.example.com",
					CAPort:       7055,
					EnrollID:     "user1",
					EnrollSecret: "password",
					CATLS: &fabricxv1alpha1.CATLSConfig{
						SecretRef: &fabricxv1alpha1.SecretRef{
							Name:      "ca-tls",
							Key:       "ca.pem",
							Namespace: "fabric",
						},
					},
				},
			},
		}

		providerConfig, err := ConvertCertificateConfigToProviderConfig(apiConfig)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if providerConfig.Type != "fabric-ca" {
			t.Errorf("Expected type 'fabric-ca', got '%s'", providerConfig.Type)
		}

		if providerConfig.FabricCA.CAHost != "ca.new.example.com" {
			t.Errorf("Expected CAHost 'ca.new.example.com', got '%s'", providerConfig.FabricCA.CAHost)
		}

		if providerConfig.FabricCA.CATLS.SecretRef == nil {
			t.Fatal("SecretRef is nil")
		}

		if providerConfig.FabricCA.CATLS.SecretRef.Name != "ca-tls" {
			t.Errorf("Expected SecretRef.Name 'ca-tls', got '%s'", providerConfig.FabricCA.CATLS.SecretRef.Name)
		}
	})

	t.Run("Manual provider", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.CertificateConfig{
			ProviderType: "manual",
			Provider: &fabricxv1alpha1.CertificateProviderConfig{
				Manual: &fabricxv1alpha1.ManualProviderConfig{
					SecretRef: &fabricxv1alpha1.SecretRef{
						Name:      "my-cert",
						Namespace: "default",
					},
					CertKey: "tls.crt",
					KeyKey:  "tls.key",
					CAKey:   "ca.crt",
				},
			},
		}

		providerConfig, err := ConvertCertificateConfigToProviderConfig(apiConfig)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if providerConfig.Type != "manual" {
			t.Errorf("Expected type 'manual', got '%s'", providerConfig.Type)
		}

		if providerConfig.Manual == nil {
			t.Fatal("Manual config is nil")
		}

		if providerConfig.Manual.SecretRef.Name != "my-cert" {
			t.Errorf("Expected SecretRef.Name 'my-cert', got '%s'", providerConfig.Manual.SecretRef.Name)
		}

		if providerConfig.Manual.CertKey != "tls.crt" {
			t.Errorf("Expected CertKey 'tls.crt', got '%s'", providerConfig.Manual.CertKey)
		}
	})

	t.Run("Vault provider", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.CertificateConfig{
			ProviderType: "vault",
			Provider: &fabricxv1alpha1.CertificateProviderConfig{
				Vault: &fabricxv1alpha1.VaultProviderConfig{
					Address:        "https://vault.example.com:8200",
					PKIPath:        "fabric-pki",
					Role:           "fabric-component",
					AuthMethod:     "kubernetes",
					ServiceAccount: "fabric-operator",
					TTL:            "8760h",
					TokenSecretRef: &fabricxv1alpha1.SecretRef{
						Name:      "vault-token",
						Key:       "token",
						Namespace: "vault",
					},
				},
			},
		}

		providerConfig, err := ConvertCertificateConfigToProviderConfig(apiConfig)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if providerConfig.Type != "vault" {
			t.Errorf("Expected type 'vault', got '%s'", providerConfig.Type)
		}

		if providerConfig.Vault == nil {
			t.Fatal("Vault config is nil")
		}

		if providerConfig.Vault.Address != "https://vault.example.com:8200" {
			t.Errorf("Expected Address 'https://vault.example.com:8200', got '%s'", providerConfig.Vault.Address)
		}

		if providerConfig.Vault.TTL != "8760h" {
			t.Errorf("Expected TTL '8760h', got '%s'", providerConfig.Vault.TTL)
		}

		if providerConfig.Vault.TokenSecretRef == nil {
			t.Fatal("TokenSecretRef is nil")
		}
	})

	t.Run("Error when provider type doesn't match config", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.CertificateConfig{
			ProviderType: "fabric-ca",
			Provider: &fabricxv1alpha1.CertificateProviderConfig{
				Manual: &fabricxv1alpha1.ManualProviderConfig{
					SecretRef: &fabricxv1alpha1.SecretRef{
						Name: "test",
					},
				},
			},
		}

		_, err := ConvertCertificateConfigToProviderConfig(apiConfig)
		if err == nil {
			t.Fatal("Expected error for mismatched provider type, got nil")
		}

		if err.Error() != "fabric-ca provider requires CA configuration" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("Nil config returns nil", func(t *testing.T) {
		providerConfig, err := ConvertCertificateConfigToProviderConfig(nil)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		if providerConfig != nil {
			t.Error("Expected nil provider config")
		}
	})

	t.Run("Infer provider type from Provider field", func(t *testing.T) {
		// No ProviderType specified, should infer from Provider.Manual
		apiConfig := &fabricxv1alpha1.CertificateConfig{
			Provider: &fabricxv1alpha1.CertificateProviderConfig{
				Manual: &fabricxv1alpha1.ManualProviderConfig{
					SecretRef: &fabricxv1alpha1.SecretRef{
						Name: "test-cert",
					},
				},
			},
		}

		providerConfig, err := ConvertCertificateConfigToProviderConfig(apiConfig)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if providerConfig.Type != "manual" {
			t.Errorf("Expected inferred type 'manual', got '%s'", providerConfig.Type)
		}
	})

	t.Run("Default to manual when no config", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.CertificateConfig{}

		_, err := ConvertCertificateConfigToProviderConfig(apiConfig)
		if err == nil {
			t.Fatal("Expected error for missing configuration")
		}
	})
}

func TestConvertFabricCAProviderConfig(t *testing.T) {
	t.Run("Full config with inline cert", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.FabricCAProviderConfig{
			CAHost:       "ca.test.com",
			CAPort:       7054,
			CAName:       "test-ca",
			EnrollID:     "admin",
			EnrollSecret: "adminpw",
			CATLS: &fabricxv1alpha1.CATLSConfig{
				CACert: "base64encodedcert",
			},
		}

		config := convertFabricCAProviderConfig(apiConfig)

		if config.CAHost != "ca.test.com" {
			t.Errorf("Expected CAHost 'ca.test.com', got '%s'", config.CAHost)
		}

		if config.CAPort != 7054 {
			t.Errorf("Expected CAPort 7054, got %d", config.CAPort)
		}

		if config.CATLS.CACert != "base64encodedcert" {
			t.Errorf("Expected CACert 'base64encodedcert', got '%s'", config.CATLS.CACert)
		}
	})

	t.Run("With SecretRef", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.FabricCAProviderConfig{
			CAHost:       "ca.test.com",
			CAPort:       7055,
			EnrollID:     "user1",
			EnrollSecret: "pass",
			CATLS: &fabricxv1alpha1.CATLSConfig{
				SecretRef: &fabricxv1alpha1.SecretRef{
					Name:      "ca-cert",
					Key:       "ca.pem",
					Namespace: "fabric",
				},
			},
		}

		config := convertFabricCAProviderConfig(apiConfig)

		if config.CATLS.SecretRef == nil {
			t.Fatal("SecretRef is nil")
		}

		if config.CATLS.SecretRef.Name != "ca-cert" {
			t.Errorf("Expected SecretRef.Name 'ca-cert', got '%s'", config.CATLS.SecretRef.Name)
		}
	})

	t.Run("Nil config returns nil", func(t *testing.T) {
		config := convertFabricCAProviderConfig(nil)
		if config != nil {
			t.Error("Expected nil config")
		}
	})
}

func TestConvertManualProviderConfig(t *testing.T) {
	t.Run("Full config", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.ManualProviderConfig{
			SecretRef: &fabricxv1alpha1.SecretRef{
				Name:      "my-cert",
				Namespace: "default",
			},
			CertKey: "tls.crt",
			KeyKey:  "tls.key",
			CAKey:   "ca.crt",
		}

		config := convertManualProviderConfig(apiConfig)

		if config.SecretRef.Name != "my-cert" {
			t.Errorf("Expected SecretRef.Name 'my-cert', got '%s'", config.SecretRef.Name)
		}

		if config.CertKey != "tls.crt" {
			t.Errorf("Expected CertKey 'tls.crt', got '%s'", config.CertKey)
		}
	})

	t.Run("Nil config returns nil", func(t *testing.T) {
		config := convertManualProviderConfig(nil)
		if config != nil {
			t.Error("Expected nil config")
		}
	})
}

func TestConvertVaultProviderConfig(t *testing.T) {
	t.Run("Full config", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.VaultProviderConfig{
			Address:        "https://vault.test.com:8200",
			PKIPath:        "pki",
			Role:           "fabric",
			AuthMethod:     "kubernetes",
			ServiceAccount: "operator",
			Namespace:      "vault-ns",
			TTL:            "8760h",
			TokenSecretRef: &fabricxv1alpha1.SecretRef{
				Name:      "vault-token",
				Key:       "token",
				Namespace: "vault",
			},
		}

		config := convertVaultProviderConfig(apiConfig)

		if config.Address != "https://vault.test.com:8200" {
			t.Errorf("Expected Address 'https://vault.test.com:8200', got '%s'", config.Address)
		}

		if config.TTL != "8760h" {
			t.Errorf("Expected TTL '8760h', got '%s'", config.TTL)
		}

		if config.TokenSecretRef == nil {
			t.Fatal("TokenSecretRef is nil")
		}

		if config.TokenSecretRef.Name != "vault-token" {
			t.Errorf("Expected TokenSecretRef.Name 'vault-token', got '%s'", config.TokenSecretRef.Name)
		}
	})

	t.Run("Nil config returns nil", func(t *testing.T) {
		config := convertVaultProviderConfig(nil)
		if config != nil {
			t.Error("Expected nil config")
		}
	})
}

func TestConvertLegacyCAConfig(t *testing.T) {
	t.Run("Full legacy config", func(t *testing.T) {
		apiConfig := &fabricxv1alpha1.CACertificateConfig{
			CAHost:       "ca.legacy.com",
			CAPort:       7056,
			CAName:       "legacy-ca",
			EnrollID:     "admin",
			EnrollSecret: "password",
			CATLS: &fabricxv1alpha1.CATLSConfig{
				CACert: "legacycert",
			},
		}

		config := convertLegacyCAConfig(apiConfig)

		if config.CAHost != "ca.legacy.com" {
			t.Errorf("Expected CAHost 'ca.legacy.com', got '%s'", config.CAHost)
		}

		if config.CAPort != 7056 {
			t.Errorf("Expected CAPort 7056, got %d", config.CAPort)
		}

		if config.CATLS.CACert != "legacycert" {
			t.Errorf("Expected CACert 'legacycert', got '%s'", config.CATLS.CACert)
		}
	})

	t.Run("Nil config returns nil", func(t *testing.T) {
		config := convertLegacyCAConfig(nil)
		if config != nil {
			t.Error("Expected nil config")
		}
	})
}
