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
	assert.Equal(t, "internal", appOrg.Type)
	assert.NotNil(t, appOrg.Internal)
	assert.Nil(t, appOrg.External)

	// Verify internal configuration
	assert.Equal(t, "AppOrg1-ca", appOrg.Internal.CAReference.Name)
	assert.Equal(t, "default", appOrg.Internal.CAReference.Namespace)
	assert.Equal(t, "admin", appOrg.Internal.AdminIdentity)
	assert.Equal(t, "peer", appOrg.Internal.PeerIdentity)

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
	assert.Equal(t, "external", appOrg.Type)
	assert.Nil(t, appOrg.Internal)
	assert.NotNil(t, appOrg.External)

	// Verify external configuration
	assert.NotEmpty(t, appOrg.External.SignCACert)
	assert.NotEmpty(t, appOrg.External.TLSCACert)
	assert.NotEmpty(t, appOrg.External.AdminCert)
	assert.NotEmpty(t, appOrg.External.PeerCert)

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
	assert.Equal(t, "OrdererOrg-ca", ordererOrg.CAReference.Name)
	assert.Equal(t, "default", ordererOrg.CAReference.Namespace)
	assert.Equal(t, "admin", ordererOrg.AdminIdentity)
	assert.Equal(t, "orderer", ordererOrg.OrdererIdentity)

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
	assert.NotEmpty(t, externalOrg.SignCert)
	assert.NotEmpty(t, externalOrg.TLSCert)
	assert.NotEmpty(t, externalOrg.AdminCert)
	assert.NotEmpty(t, externalOrg.OrdererCert)

	// Verify certificates are generated
	assert.NotEmpty(t, certBundle.CA)
	assert.NotEmpty(t, certBundle.TLS)
	assert.NotEmpty(t, certBundle.Admin)
	assert.NotEmpty(t, certBundle.Orderer)
}

func TestGenerateOrdererNodes(t *testing.T) {
	nodes, certBundle, err := GenerateOrdererNodes("OrdererOrgMSP", 3, "orderer", 7050)

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
		assert.NotEmpty(t, node.ClientTLSCert)
		assert.NotEmpty(t, node.ServerTLSCert)
		assert.NotEmpty(t, node.Identity)
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

func TestGenerateCompleteGenesis(t *testing.T) {
	// Generate application organizations
	appOrg1, _, err := GenerateApplicationOrganization("AppOrg1", "AppOrg1MSP", "internal")
	require.NoError(t, err)

	appOrg2, _, err := GenerateApplicationOrganization("AppOrg2", "AppOrg2MSP", "external")
	require.NoError(t, err)

	// Generate orderer organization
	ordererOrg, _, err := GenerateOrdererOrganization("OrdererOrg", "OrdererOrgMSP")
	require.NoError(t, err)

	// Generate external organization
	externalOrg, _, err := GenerateExternalOrganization("ExternalOrg", "ExternalOrgMSP")
	require.NoError(t, err)

	// Generate orderer nodes
	ordererNodes, _, err := GenerateOrdererNodes("OrdererOrgMSP", 2, "orderer", 7050)
	require.NoError(t, err)

	// Create complete genesis
	genesis := &v1alpha1.Genesis{
		Spec: v1alpha1.GenesisSpec{
			InternalOrgs:    []v1alpha1.InternalOrganization{*ordererOrg},
			ExternalOrgs:    []v1alpha1.ExternalOrganization{*externalOrg},
			ApplicationOrgs: []v1alpha1.ApplicationOrganization{*appOrg1, *appOrg2},
			OrdererNodes:    ordererNodes,
			Output: v1alpha1.GenesisOutput{
				SecretName: "genesis-block-secret",
				BlockKey:   "genesis.block",
			},
		},
	}

	// Verify genesis structure
	assert.Len(t, genesis.Spec.InternalOrgs, 1)
	assert.Len(t, genesis.Spec.ExternalOrgs, 1)
	assert.Len(t, genesis.Spec.ApplicationOrgs, 2)
	assert.Len(t, genesis.Spec.OrdererNodes, 2)

	// Verify internal orgs
	assert.Equal(t, "OrdererOrg", genesis.Spec.InternalOrgs[0].Name)
	assert.Equal(t, "OrdererOrgMSP", genesis.Spec.InternalOrgs[0].MSPID)

	// Verify external orgs
	assert.Equal(t, "ExternalOrg", genesis.Spec.ExternalOrgs[0].Name)
	assert.Equal(t, "ExternalOrgMSP", genesis.Spec.ExternalOrgs[0].MSPID)

	// Verify application orgs
	assert.Equal(t, "AppOrg1", genesis.Spec.ApplicationOrgs[0].Name)
	assert.Equal(t, "AppOrg1MSP", genesis.Spec.ApplicationOrgs[0].MSPID)
	assert.Equal(t, "internal", genesis.Spec.ApplicationOrgs[0].Type)

	assert.Equal(t, "AppOrg2", genesis.Spec.ApplicationOrgs[1].Name)
	assert.Equal(t, "AppOrg2MSP", genesis.Spec.ApplicationOrgs[1].MSPID)
	assert.Equal(t, "external", genesis.Spec.ApplicationOrgs[1].Type)

	// Verify orderer nodes
	assert.Equal(t, 1, genesis.Spec.OrdererNodes[0].ID)
	assert.Equal(t, "orderer1", genesis.Spec.OrdererNodes[0].Host)
	assert.Equal(t, 7050, genesis.Spec.OrdererNodes[0].Port)

	assert.Equal(t, 2, genesis.Spec.OrdererNodes[1].ID)
	assert.Equal(t, "orderer2", genesis.Spec.OrdererNodes[1].Host)
	assert.Equal(t, 7051, genesis.Spec.OrdererNodes[1].Port)
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
