package genesis

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"

	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateOrganizationCertificates(t *testing.T) {
	config := DefaultOrganizationConfig("TestOrg", "TestOrgMSP")
	certBundle, err := GenerateOrganizationCertificates(config)

	require.NoError(t, err)
	assert.NotNil(t, certBundle)

	// Verify all certificates are generated
	assert.NotEmpty(t, certBundle.CA)
	assert.NotEmpty(t, certBundle.TLS)
	assert.NotEmpty(t, certBundle.Admin)
	assert.NotEmpty(t, certBundle.Peer)
	assert.NotEmpty(t, certBundle.Orderer)
	assert.NotEmpty(t, certBundle.Consenter)
	assert.NotEmpty(t, certBundle.Router)
	assert.NotEmpty(t, certBundle.Batcher)
	assert.NotEmpty(t, certBundle.Assembler)

	// Verify certificates are valid PEM
	verifyPEMCertificate(t, certBundle.CA, "CA")
	verifyPEMCertificate(t, certBundle.TLS, "TLS")
	verifyPEMCertificate(t, certBundle.Admin, "Admin")
	verifyPEMCertificate(t, certBundle.Peer, "Peer")
	verifyPEMCertificate(t, certBundle.Orderer, "Orderer")
	verifyPEMCertificate(t, certBundle.Consenter, "Consenter")
	verifyPEMCertificate(t, certBundle.Router, "Router")
	verifyPEMCertificate(t, certBundle.Batcher, "Batcher")
	verifyPEMCertificate(t, certBundle.Assembler, "Assembler")
}

func TestGenerateApplicationOrganization_Internal(t *testing.T) {
	appOrg, certBundle, err := GenerateApplicationOrganization("AppOrg1", "AppOrg1MSP", "internal")

	require.NoError(t, err)
	assert.NotNil(t, appOrg)
	assert.NotNil(t, certBundle)

	// Verify organization structure
	assert.Equal(t, "AppOrg1", appOrg.Name)
	assert.Equal(t, "AppOrg1MSP", appOrg.MSPID)

	// Verify certificate references are set
	assert.NotEmpty(t, appOrg.SignCACertRef)
	assert.NotEmpty(t, appOrg.TLSCACertRef)
	assert.NotNil(t, appOrg.AdminCertRef)

	// Verify certificates are generated
	assert.NotEmpty(t, certBundle.CA)
	assert.NotEmpty(t, certBundle.TLS)
	assert.NotEmpty(t, certBundle.Admin)
	assert.NotEmpty(t, certBundle.Peer)
}

func TestGenerateApplicationOrganization_External(t *testing.T) {
	appOrg, certBundle, err := GenerateApplicationOrganization("AppOrg2", "AppOrg2MSP", "external")

	require.NoError(t, err)
	assert.NotNil(t, appOrg)
	assert.NotNil(t, certBundle)

	// Verify organization structure
	assert.Equal(t, "AppOrg2", appOrg.Name)
	assert.Equal(t, "AppOrg2MSP", appOrg.MSPID)

	// Verify certificate references are set
	assert.NotEmpty(t, appOrg.SignCACertRef)
	assert.NotEmpty(t, appOrg.TLSCACertRef)
	assert.NotNil(t, appOrg.AdminCertRef)

	// Verify certificates are generated
	assert.NotEmpty(t, certBundle.CA)
	assert.NotEmpty(t, certBundle.TLS)
	assert.NotEmpty(t, certBundle.Admin)
	assert.NotEmpty(t, certBundle.Peer)
}

func TestGenerateOrdererOrganization(t *testing.T) {
	ordererOrg, certBundle, err := GenerateOrdererOrganization("OrdererOrg", "OrdererOrgMSP")

	require.NoError(t, err)
	assert.NotNil(t, ordererOrg)
	assert.NotNil(t, certBundle)

	// Verify organization structure
	assert.Equal(t, "OrdererOrg", ordererOrg.Name)
	assert.Equal(t, "OrdererOrgMSP", ordererOrg.MSPID)

	// Verify certificate references are set
	assert.NotEmpty(t, ordererOrg.SignCACertRef)
	assert.NotEmpty(t, ordererOrg.TLSCACertRef)
	assert.NotNil(t, ordererOrg.AdminCertRef)

	// Verify certificates are generated
	assert.NotEmpty(t, certBundle.CA)
	assert.NotEmpty(t, certBundle.TLS)
	assert.NotEmpty(t, certBundle.Admin)
	assert.NotEmpty(t, certBundle.Orderer)
	assert.NotEmpty(t, certBundle.Consenter)
}

func TestGenerateExternalOrganization(t *testing.T) {
	externalOrg, certBundle, err := GenerateExternalOrganization("ExternalOrg", "ExternalOrgMSP")

	require.NoError(t, err)
	assert.NotNil(t, externalOrg)
	assert.NotNil(t, certBundle)

	// Verify organization structure
	assert.Equal(t, "ExternalOrg", externalOrg.Name)
	assert.Equal(t, "ExternalOrgMSP", externalOrg.MSPID)

	// Verify certificate references are set
	assert.NotEmpty(t, externalOrg.SignCACertRef)
	assert.NotEmpty(t, externalOrg.TLSCACertRef)
	assert.NotNil(t, externalOrg.AdminCertRef)

	// Verify certificates are generated
	assert.NotEmpty(t, certBundle.CA)
	assert.NotEmpty(t, certBundle.TLS)
	assert.NotEmpty(t, certBundle.Admin)
	assert.NotEmpty(t, certBundle.Orderer)
}

func TestGenerateConsenters(t *testing.T) {
	nodes, certBundle, err := GenerateConsenters("OrdererOrgMSP", 3, "orderer", 7050)

	require.NoError(t, err)
	assert.NotNil(t, nodes)
	assert.NotNil(t, certBundle)
	assert.Len(t, nodes, 3)

	// Verify each node
	for i, node := range nodes {
		assert.Equal(t, i+1, node.ID)
		assert.Equal(t, fmt.Sprintf("orderer%d", i+1), node.Host)
		assert.Equal(t, 7050+i, node.Port)
		assert.Equal(t, "OrdererOrgMSP", node.MSPID)
		assert.NotEmpty(t, node.ClientTLSCertRef)
		assert.NotEmpty(t, node.ServerTLSCertRef)
		assert.NotEmpty(t, node.IdentityRef)
	}

	// Verify certificates are generated
	assert.NotEmpty(t, certBundle.CA)
	assert.NotEmpty(t, certBundle.TLS)
	assert.NotEmpty(t, certBundle.Consenter)
}

func TestDefaultOrganizationConfig(t *testing.T) {
	config := DefaultOrganizationConfig("TestOrg", "TestOrgMSP")

	assert.Equal(t, "TestOrg", config.Name)
	assert.Equal(t, "TestOrgMSP", config.MSPID)
	assert.Equal(t, "US", config.Country)
	assert.Equal(t, "California", config.State)
	assert.Equal(t, "San Francisco", config.Locality)
	assert.Equal(t, "TestOrg", config.Organization)
	assert.Equal(t, "Hyperledger Fabric", config.OrganizationalUnit)
	assert.Equal(t, "TestOrg", config.CommonName)
	assert.Equal(t, 365, config.ValidDays)
}

func TestGenerateGenesisSpec(t *testing.T) {
	// Generate orderer nodes
	consenters, _, err := GenerateConsenters("OrdererOrgMSP", 2, "orderer", 7050)
	require.NoError(t, err)

	// Create genesis spec
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{
			ChannelID: "arma",
			ConfigTemplate: v1alpha1.ConfigTemplateReference{
				ConfigMapName: "config-template",
				Key:           "configtx.yaml",
			},
			OrdererOrganizations: []v1alpha1.OrdererOrganization{
				{
					Name:  "OrdererOrg",
					MSPID: "OrdererOrgMSP",
					SignCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "orderer-ca-crypto",
						Key:       "certfile",
						Namespace: "default",
					},
					TLSCACertRef: v1alpha1.SecretKeyNSSelector{
						Name:      "orderer-tlsca-crypto",
						Key:       "certfile",
						Namespace: "default",
					},
				},
			},
			Consenters: consenters,
			Output: v1alpha1.GenesisOutput{
				SecretName: "genesis-block",
				BlockKey:   "genesis.block",
			},
		},
	}

	// Verify the genesis spec
	assert.Len(t, genesis.Spec.Consenters, 2)
	assert.Equal(t, "OrdererOrgMSP", genesis.Spec.Consenters[0].MSPID)
	assert.Equal(t, 1, genesis.Spec.Consenters[0].ID)
	assert.Equal(t, "orderer1", genesis.Spec.Consenters[0].Host)
	assert.Equal(t, 7050, genesis.Spec.Consenters[0].Port)
	assert.Equal(t, "OrdererOrgMSP", genesis.Spec.Consenters[1].MSPID)
	assert.Equal(t, 2, genesis.Spec.Consenters[1].ID)
	assert.Equal(t, "orderer2", genesis.Spec.Consenters[1].Host)
	assert.Equal(t, 7051, genesis.Spec.Consenters[1].Port)
}

// verifyPEMCertificate verifies that a certificate is valid PEM format
func verifyPEMCertificate(t *testing.T, certBytes []byte, certType string) {
	block, rest := pem.Decode(certBytes)
	assert.NotNil(t, block, "%s certificate should be valid PEM", certType)
	assert.Empty(t, rest, "%s certificate should not have trailing data", certType)
	assert.Equal(t, "CERTIFICATE", block.Type, "%s certificate should have CERTIFICATE type", certType)

	// Verify it can be parsed as an X.509 certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	assert.NoError(t, err, "%s certificate should be valid X.509", certType)
	assert.NotNil(t, cert, "%s certificate should not be nil", certType)
}
