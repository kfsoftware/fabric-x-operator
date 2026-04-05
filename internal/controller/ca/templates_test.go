package ca

import (
	"fmt"
	"strings"
	"testing"

	fabricxv1alpha1 "github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

func TestGenerateConfigFromTemplate(t *testing.T) {
	// Create a test CA with users
	ca := &fabricxv1alpha1.CA{
		Spec: fabricxv1alpha1.CASpec{
			Debug:        true,
			CLRSizeLimit: 512000,
			Database: fabricxv1alpha1.FabricCADatabase{
				Type:       "sqlite3",
				Datasource: "/var/hyperledger/fabric-ca/fabric-ca-server.db",
			},
			Metrics: fabricxv1alpha1.FabricCAMetrics{
				Provider: "prometheus",
				Statsd: fabricxv1alpha1.FabricCAMetricsStatsd{
					Network:       "udp",
					Address:       "127.0.0.1:8125",
					WriteInterval: "10s",
					Prefix:        "fabric-ca",
				},
			},
			CA: fabricxv1alpha1.FabricCAItemConf{
				Name: "ca",
				Registry: fabricxv1alpha1.FabricCAItemRegistry{
					MaxEnrollments: -1,
					Identities: []fabricxv1alpha1.FabricCAIdentity{
						{
							Name:        "admin",
							Pass:        "adminpw",
							Type:        "client",
							Affiliation: "",
							Attrs: fabricxv1alpha1.FabricCAIdentityAttrs{
								RegistrarRoles: "*",
								DelegateRoles:  "*",
								Attributes:     "*",
								Revoker:        true,
								IntermediateCA: true,
								GenCRL:         true,
								AffiliationMgr: true,
							},
						},
						{
							Name:        "user1",
							Pass:        "user1pw",
							Type:        "client",
							Affiliation: "org1",
							Attrs: fabricxv1alpha1.FabricCAIdentityAttrs{
								RegistrarRoles: "",
								DelegateRoles:  "",
								Attributes:     "",
								Revoker:        false,
								IntermediateCA: false,
								GenCRL:         false,
								AffiliationMgr: false,
							},
						},
					},
				},

				CSR: fabricxv1alpha1.FabricCACSR{
					CN: "fabric-ca-server",
					Hosts: []string{
						"localhost",
						"ca.example.com",
					},
					Names: []fabricxv1alpha1.FabricCANames{
						{
							C:  "US",
							ST: "North Carolina",
							L:  "Raleigh",
							O:  "Hyperledger",
							OU: "Fabric",
						},
					},
					CA: fabricxv1alpha1.FabricCACSRCA{
						Expiry:     "131400h",
						PathLength: 0,
					},
				},
				BCCSP: fabricxv1alpha1.FabricCAItemBCCSP{
					Default: "SW",
					SW: fabricxv1alpha1.FabricCAItemBCCSPSW{
						Hash:     "SHA2",
						Security: 256,
					},
				},
				Intermediate: fabricxv1alpha1.FabricCAItemIntermediate{
					ParentServer: fabricxv1alpha1.FabricCAItemIntermediateParentServer{
						URL:    "",
						CAName: "",
					},
				},
				CFG: fabricxv1alpha1.FabricCAItemCFG{
					Identities: fabricxv1alpha1.FabricCAItemCFGIdentities{
						AllowRemove: false,
					},
					Affiliations: fabricxv1alpha1.FabricCAItemCFGAffiliations{
						AllowRemove: false,
					},
				},
			},
			TLSCA: fabricxv1alpha1.FabricCAItemConf{
				Name: "tlsca",
				Registry: fabricxv1alpha1.FabricCAItemRegistry{
					MaxEnrollments: -1,
					Identities: []fabricxv1alpha1.FabricCAIdentity{
						{
							Name:        "admin",
							Pass:        "adminpw",
							Type:        "client",
							Affiliation: "",
							Attrs: fabricxv1alpha1.FabricCAIdentityAttrs{
								RegistrarRoles: "*",
								DelegateRoles:  "*",
								Attributes:     "*",
								Revoker:        true,
								IntermediateCA: true,
								GenCRL:         true,
								AffiliationMgr: true,
							},
						},
					},
				},
				CSR: fabricxv1alpha1.FabricCACSR{
					CN: "fabric-tlsca-server",
					Hosts: []string{
						"localhost",
						"tlsca.example.com",
					},
					Names: []fabricxv1alpha1.FabricCANames{
						{
							C:  "US",
							ST: "North Carolina",
							L:  "Raleigh",
							O:  "Hyperledger",
							OU: "Fabric",
						},
					},
					CA: fabricxv1alpha1.FabricCACSRCA{
						Expiry:     "131400h",
						PathLength: 0,
					},
				},
				BCCSP: fabricxv1alpha1.FabricCAItemBCCSP{
					Default: "SW",
					SW: fabricxv1alpha1.FabricCAItemBCCSPSW{
						Hash:     "SHA2",
						Security: 256,
					},
				},
				Intermediate: fabricxv1alpha1.FabricCAItemIntermediate{
					ParentServer: fabricxv1alpha1.FabricCAItemIntermediateParentServer{
						URL:    "",
						CAName: "",
					},
				},
				CFG: fabricxv1alpha1.FabricCAItemCFG{
					Identities: fabricxv1alpha1.FabricCAItemCFGIdentities{
						AllowRemove: false,
					},
					Affiliations: fabricxv1alpha1.FabricCAItemCFGAffiliations{
						AllowRemove: false,
					},
				},
			},
		},
	}

	// Test CA config generation
	caConfig, err := generateCAConfigForTest(ca)
	if err != nil {
		t.Errorf("Failed to generate CA config: %v", err)
	}
	fmt.Println(caConfig)
	if !strings.Contains(caConfig, "admin") {
		t.Error("Expected CA config to contain admin user")
	}
	if !strings.Contains(caConfig, "user1") {
		t.Error("Expected CA config to contain user1")
	}
	if !strings.Contains(caConfig, "org1") {
		t.Error("Expected CA config to contain org1 affiliation")
	}

	// Test TLS CA config generation
	tlsConfig, err := generateTLSCAConfigForTest(ca)
	if err != nil {
		t.Errorf("Failed to generate TLS CA config: %v", err)
	}
	if !strings.Contains(tlsConfig, "admin") {
		t.Error("Expected TLS CA config to contain admin user")
	}
	if !strings.Contains(tlsConfig, "tlsca") {
		t.Error("Expected TLS CA config to contain tlsca name")
	}

	// Test that both configs contain required sections
	requiredSections := []string{
		"version:",
		"debug:",
		"tls:",
		"ca:",
		"registry:",
		"db:",
		"signing:",
		"csr:",
		"bccsp:",
	}

	for _, section := range requiredSections {
		if !strings.Contains(caConfig, section) {
			t.Errorf("Expected CA config to contain section: %s", section)
		}
		if !strings.Contains(tlsConfig, section) {
			t.Errorf("Expected TLS CA config to contain section: %s", section)
		}
	}
}

// Helper functions for testing
func generateCAConfigForTest(ca *fabricxv1alpha1.CA) (string, error) {
	// Prepare data for template
	data := ConfigData{
		Debug:        ca.Spec.Debug,
		CLRSizeLimit: ca.Spec.CLRSizeLimit,
		Database: struct {
			Type       string
			Datasource string
		}{
			Type:       ca.Spec.Database.Type,
			Datasource: ca.Spec.Database.Datasource,
		},
		Metrics: struct {
			Provider string
			Statsd   struct {
				Network       string
				Address       string
				WriteInterval string
				Prefix        string
			}
		}{
			Provider: ca.Spec.Metrics.Provider,
			Statsd: struct {
				Network       string
				Address       string
				WriteInterval string
				Prefix        string
			}{
				Network:       ca.Spec.Metrics.Statsd.Network,
				Address:       ca.Spec.Metrics.Statsd.Address,
				WriteInterval: ca.Spec.Metrics.Statsd.WriteInterval,
				Prefix:        ca.Spec.Metrics.Statsd.Prefix,
			},
		},
		CA:    ca.Spec.CA,
		TLSCA: ca.Spec.TLSCA,
	}

	config, err := GenerateConfigFromTemplate(CAConfigTemplate, data)
	if err != nil {
		return "", err
	}
	return config, nil
}

func generateTLSCAConfigForTest(ca *fabricxv1alpha1.CA) (string, error) {
	// Prepare data for template
	data := ConfigData{
		Debug:        ca.Spec.Debug,
		CLRSizeLimit: ca.Spec.CLRSizeLimit,
		Database: struct {
			Type       string
			Datasource string
		}{
			Type:       ca.Spec.Database.Type,
			Datasource: ca.Spec.Database.Datasource,
		},
		Metrics: struct {
			Provider string
			Statsd   struct {
				Network       string
				Address       string
				WriteInterval string
				Prefix        string
			}
		}{
			Provider: ca.Spec.Metrics.Provider,
			Statsd: struct {
				Network       string
				Address       string
				WriteInterval string
				Prefix        string
			}{
				Network:       ca.Spec.Metrics.Statsd.Network,
				Address:       ca.Spec.Metrics.Statsd.Address,
				WriteInterval: ca.Spec.Metrics.Statsd.WriteInterval,
				Prefix:        ca.Spec.Metrics.Statsd.Prefix,
			},
		},
		CA:    ca.Spec.CA,
		TLSCA: ca.Spec.TLSCA,
	}

	config, err := GenerateConfigFromTemplate(TLSConfigTemplate, data)
	if err != nil {
		return "", err
	}
	return config, nil
}
