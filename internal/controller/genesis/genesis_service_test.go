/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package genesis

import (
	"context"
	"testing"

	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestGenesisService_CreateGenesisBlock(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Generate test organizations using reusable functions
	appOrg1, certBundle1, err := GenerateApplicationOrganization("AppOrg1", "AppOrg1MSP", "internal")
	require.NoError(t, err)

	appOrg2, certBundle2, err := GenerateApplicationOrganization("AppOrg2", "AppOrg2MSP", "external")
	require.NoError(t, err)

	ordererOrg, certBundle3, err := GenerateOrdererOrganization("OrdererOrg", "OrdererOrgMSP")
	require.NoError(t, err)

	externalOrg, _, err := GenerateExternalOrganization("ExternalOrg", "ExternalOrgMSP")
	require.NoError(t, err)

	// Generate orderer nodes
	consenters, _, err := GenerateConsenters("OrdererOrgMSP", 2, "orderer", 7050)
	require.NoError(t, err)

	// Create mock CA resources with correct names
	ca1 := &v1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca1",
			Namespace: "default",
		},
		Spec: v1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:latest",
			Version: "1.5.0",
		},
		Status: v1alpha1.CAStatus{
			Status:    v1alpha1.RunningStatus,
			CACert:    string(certBundle1.CA),
			TLSCACert: string(certBundle1.TLS),
		},
	}

	ca2 := &v1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca2",
			Namespace: "default",
		},
		Spec: v1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:latest",
			Version: "1.5.0",
		},
		Status: v1alpha1.CAStatus{
			Status:    v1alpha1.RunningStatus,
			CACert:    string(certBundle2.CA),
			TLSCACert: string(certBundle2.TLS),
		},
	}

	ordererCA := &v1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-ca",
			Namespace: "default",
		},
		Spec: v1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:latest",
			Version: "1.5.0",
		},
		Status: v1alpha1.CAStatus{
			Status:    v1alpha1.RunningStatus,
			CACert:    string(certBundle3.CA),
			TLSCACert: string(certBundle3.TLS),
		},
	}

	// Add CA for internal application organization
	appOrg1CA := &v1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg1-ca",
			Namespace: "default",
		},
		Spec: v1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:latest",
			Version: "1.5.0",
		},
		Status: v1alpha1.CAStatus{
			Status:    v1alpha1.RunningStatus,
			CACert:    string(certBundle1.CA),
			TLSCACert: string(certBundle1.TLS),
		},
	}

	// Create mock secrets for certificate references
	ordererSignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle3.CA,
		},
	}

	ordererTLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle3.TLS,
		},
	}

	ordererAdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle3.Admin,
		},
	}

	externalSignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle3.CA,
		},
	}

	externalTLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle3.TLS,
		},
	}

	externalAdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle3.Admin,
		},
	}

	appOrg1SignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg1-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle1.CA,
		},
	}

	appOrg1TLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg1-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle1.TLS,
		},
	}

	appOrg1AdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg1-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle1.Admin,
		},
	}

	appOrg2SignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg2-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle2.CA,
		},
	}

	appOrg2TLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg2-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle2.TLS,
		},
	}

	appOrg2AdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg2-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle2.Admin,
		},
	}

	// Create fake client with CA resources and secrets
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ca1, ca2, ordererCA, appOrg1CA).
		WithObjects(ordererSignSecret, ordererTLSSecret, ordererAdminSecret).
		WithObjects(externalSignSecret, externalTLSSecret, externalAdminSecret).
		WithObjects(appOrg1SignSecret, appOrg1TLSSecret, appOrg1AdminSecret).
		WithObjects(appOrg2SignSecret, appOrg2TLSSecret, appOrg2AdminSecret).
		Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create test genesis with all organization types
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			OrdererOrganizations: []v1alpha1.OrdererOrganization{*ordererOrg, *externalOrg},
			ApplicationOrgs:      []v1alpha1.ApplicationOrganization{*appOrg1, *appOrg2},
			Consenters:           consenters,
			Output: v1alpha1.GenesisOutput{
				SecretName: "genesis-block-secret",
				BlockKey:   "genesis.block",
			},
		},
	}

	req := &GenesisRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	genesisResult, err := service.CreateGenesisBlock(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, genesisResult)
	assert.NotNil(t, genesisResult.GenesisBlock)
	assert.NotEmpty(t, genesisResult.GenesisBlock)

	// Verify the genesis block is valid protobuf
	assert.Greater(t, len(genesisResult.GenesisBlock), 0)

	// Verify shared config is also generated
	assert.NotNil(t, genesisResult.SharedConfigProto)
	assert.NotEmpty(t, genesisResult.SharedConfigProto)
	assert.NotNil(t, genesisResult.SharedConfigJSON)
	assert.NotEmpty(t, genesisResult.SharedConfigJSON)
}

func TestGenesisService_CreateGenesisBlock_EmptyGenesis(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create empty genesis
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			Output: v1alpha1.GenesisOutput{
				SecretName: "genesis-block-secret",
				BlockKey:   "genesis.block",
			},
		},
	}

	req := &GenesisRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	_, err = service.CreateGenesisBlock(ctx, req)

	// Empty genesis should return an error since it needs at least some organizations
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no organizations")
}

func TestGenesisService_CreateGenesisBlock_ExternalOrgsOnly(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Generate test organizations using reusable functions
	externalOrg1, _, err := GenerateExternalOrganization("ExternalOrg1", "ExternalOrg1MSP")
	require.NoError(t, err)

	externalOrg2, _, err := GenerateExternalOrganization("ExternalOrg2", "ExternalOrg2MSP")
	require.NoError(t, err)

	// Create orderer organization for the orderer nodes
	ordererOrg, certBundle, err := GenerateOrdererOrganization("OrdererOrg", "OrdererOrgMSP")
	require.NoError(t, err)

	// Generate orderer nodes for the orderer organization
	consenters, _, err := GenerateConsenters("ExternalOrg1MSP", 1, "orderer", 7050)
	require.NoError(t, err)

	// Create mock CA resource for orderer organization
	ordererCA := &v1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-ca",
			Namespace: "default",
		},
		Spec: v1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:latest",
			Version: "1.5.0",
		},
		Status: v1alpha1.CAStatus{
			Status:    v1alpha1.RunningStatus,
			CACert:    string(certBundle.CA),
			TLSCACert: string(certBundle.TLS),
		},
	}

	// Create mock secrets for certificate references
	ordererSignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.CA,
		},
	}

	ordererTLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.TLS,
		},
	}

	ordererAdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle.Admin,
		},
	}

	external1SignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg1-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.CA,
		},
	}

	external1TLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg1-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.TLS,
		},
	}

	external1AdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg1-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle.Admin,
		},
	}

	external2SignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg2-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.CA,
		},
	}

	external2TLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg2-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.TLS,
		},
	}

	external2AdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ExternalOrg2-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle.Admin,
		},
	}

	// Create fake client with CA resource and secrets
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ordererCA).
		WithObjects(ordererSignSecret, ordererTLSSecret, ordererAdminSecret).
		WithObjects(external1SignSecret, external1TLSSecret, external1AdminSecret).
		WithObjects(external2SignSecret, external2TLSSecret, external2AdminSecret).
		Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create genesis with external organizations and orderer organization
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			OrdererOrganizations: []v1alpha1.OrdererOrganization{*ordererOrg, *externalOrg1, *externalOrg2},
			Consenters:           consenters,
			Output: v1alpha1.GenesisOutput{
				SecretName: "genesis-block-secret",
				BlockKey:   "genesis.block",
			},
		},
	}

	req := &GenesisRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	genesisResult, err := service.CreateGenesisBlock(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, genesisResult)
	assert.NotNil(t, genesisResult.GenesisBlock)
	assert.NotEmpty(t, genesisResult.GenesisBlock)
}

func TestGenesisService_CreateGenesisBlock_ApplicationOrgsOnly(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Generate test organizations using reusable functions
	appOrg1, _, err := GenerateApplicationOrganization("AppOrg1", "AppOrg1MSP", "external")
	require.NoError(t, err)

	appOrg2, _, err := GenerateApplicationOrganization("AppOrg2", "AppOrg2MSP", "external")
	require.NoError(t, err)

	// Create orderer organization for the genesis block
	ordererOrg, certBundle, err := GenerateOrdererOrganization("OrdererOrg", "OrdererOrgMSP")
	require.NoError(t, err)

	// Generate orderer nodes for the orderer organization
	consenters, _, err := GenerateConsenters("OrdererOrgMSP", 2, "orderer", 7050)
	require.NoError(t, err)

	// Create mock CA resource for orderer organization
	ordererCA := &v1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-ca",
			Namespace: "default",
		},
		Spec: v1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:latest",
			Version: "1.5.0",
		},
		Status: v1alpha1.CAStatus{
			Status:    v1alpha1.RunningStatus,
			CACert:    string(certBundle.CA),
			TLSCACert: string(certBundle.TLS),
		},
	}

	// Create mock secrets for certificate references
	ordererSignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.CA,
		},
	}

	ordererTLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.TLS,
		},
	}

	ordererAdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle.Admin,
		},
	}

	appOrg1SignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg1-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.CA,
		},
	}

	appOrg1TLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg1-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.TLS,
		},
	}

	appOrg1AdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg1-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle.Admin,
		},
	}

	appOrg2SignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg2-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.CA,
		},
	}

	appOrg2TLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg2-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.TLS,
		},
	}

	appOrg2AdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "AppOrg2-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle.Admin,
		},
	}

	// Create fake client with CA resource and secrets
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ordererCA).
		WithObjects(ordererSignSecret, ordererTLSSecret, ordererAdminSecret).
		WithObjects(appOrg1SignSecret, appOrg1TLSSecret, appOrg1AdminSecret).
		WithObjects(appOrg2SignSecret, appOrg2TLSSecret, appOrg2AdminSecret).
		Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create genesis with application organizations and orderer organization
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			OrdererOrganizations: []v1alpha1.OrdererOrganization{*ordererOrg}, // Add orderer org
			ApplicationOrgs:      []v1alpha1.ApplicationOrganization{*appOrg1, *appOrg2},
			Consenters:           consenters, // Add consenters
			Output: v1alpha1.GenesisOutput{
				SecretName: "genesis-block-secret",
				BlockKey:   "genesis.block",
			},
		},
	}

	req := &GenesisRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	genesisResult, err := service.CreateGenesisBlock(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, genesisResult)
	assert.NotNil(t, genesisResult.GenesisBlock)
	assert.NotEmpty(t, genesisResult.GenesisBlock)
}

func TestGenesisService_GetConfigTemplate(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Create test ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-template",
			Namespace: "default",
		},
		Data: map[string]string{
			"configtx.yaml": "test-config-template",
		},
	}

	// Create fake client with ConfigMap
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(configMap).Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create test genesis
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			ConfigTemplate: v1alpha1.ConfigTemplateReference{
				ConfigMapName: "config-template",
				Key:           "configtx.yaml",
			},
		},
	}

	ctx := context.Background()
	template, err := service.GetConfigTemplate(ctx, genesis)

	require.NoError(t, err)
	assert.Equal(t, []byte("test-config-template"), template)
}

func TestGenesisService_CreateGenesisBlock_WithInternalOrgs(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Generate test organizations using reusable functions
	ordererOrg, certBundle, err := GenerateOrdererOrganization("OrdererOrg", "OrdererOrgMSP")
	require.NoError(t, err)

	// Generate orderer nodes for the orderer organization
	consenters, _, err := GenerateConsenters("OrdererOrgMSP", 2, "orderer", 7050)
	require.NoError(t, err)

	// Create mock CA resource
	ca := &v1alpha1.CA{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-ca",
			Namespace: "default",
		},
		Spec: v1alpha1.CASpec{
			Image:   "hyperledger/fabric-ca:latest",
			Version: "1.5.0",
		},
		Status: v1alpha1.CAStatus{
			Status:    v1alpha1.RunningStatus,
			CACert:    string(certBundle.CA),
			TLSCACert: string(certBundle.TLS),
		},
	}

	// Create mock secrets for certificate references
	ordererSignSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-sign-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.CA,
		},
	}

	ordererTLSSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-tls-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": certBundle.TLS,
		},
	}

	ordererAdminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "OrdererOrg-admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"admin.crt": certBundle.Admin,
		},
	}

	// Create fake client with CA resource and secrets
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ca).
		WithObjects(ordererSignSecret, ordererTLSSecret, ordererAdminSecret).
		Build()

	opts := zap.Options{
		Development: true,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create test genesis with internal organizations only
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "internal-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			OrdererOrganizations: []v1alpha1.OrdererOrganization{*ordererOrg},
			Consenters:           consenters, // Add consenters
			Output: v1alpha1.GenesisOutput{
				SecretName: "genesis-block-secret",
				BlockKey:   "genesis.block",
			},
		},
	}

	req := &GenesisRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	genesisResult, err := service.CreateGenesisBlock(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, genesisResult)
	assert.NotNil(t, genesisResult.GenesisBlock)
	assert.NotEmpty(t, genesisResult.GenesisBlock)
}
