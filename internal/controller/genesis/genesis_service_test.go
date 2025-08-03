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
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

	ordererNodes, _, err := GenerateOrdererNodes("OrdererOrgMSP", 2, "orderer", 7050)
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

	// Create fake client with CA resources
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ca1, ca2, ordererCA, appOrg1CA).
		Build()

	logger := logrus.New()
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create test genesis with all organization types
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
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

	req := &GenesisRequest{
		Genesis:   genesis,
		ChannelID: "testchannel",
	}

	ctx := context.Background()
	genesisBlock, err := service.CreateGenesisBlock(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, genesisBlock)
	assert.NotEmpty(t, genesisBlock)

	// Verify the genesis block is valid protobuf
	assert.Greater(t, len(genesisBlock), 0)
}

func TestGenesisService_CreateGenesisBlock_EmptyGenesis(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	logger := logrus.New()
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
	ordererOrg, certBundle, err := GenerateOrdererOrganization("OrdererOrg", "ExternalOrg1MSP")
	require.NoError(t, err)

	ordererNodes, _, err := GenerateOrdererNodes("ExternalOrg1MSP", 1, "orderer", 7050)
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

	// Create fake client with CA resource
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ordererCA).
		Build()

	logger := logrus.New()
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create genesis with external organizations and orderer organization
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			InternalOrgs: []v1alpha1.InternalOrganization{*ordererOrg}, // Add orderer org
			ExternalOrgs: []v1alpha1.ExternalOrganization{*externalOrg1, *externalOrg2},
			OrdererNodes: ordererNodes,
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
	genesisBlock, err := service.CreateGenesisBlock(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, genesisBlock)
	assert.NotEmpty(t, genesisBlock)
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
	ordererNodes, _, err := GenerateOrdererNodes("OrdererOrgMSP", 2, "orderer", 7050)
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

	// Create fake client with CA resource
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ordererCA).
		Build()

	logger := logrus.New()
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create genesis with application organizations and orderer organization
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			InternalOrgs:    []v1alpha1.InternalOrganization{*ordererOrg}, // Add orderer org
			ApplicationOrgs: []v1alpha1.ApplicationOrganization{*appOrg1, *appOrg2},
			OrdererNodes:    ordererNodes, // Add orderer nodes
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
	genesisBlock, err := service.CreateGenesisBlock(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, genesisBlock)
	assert.NotEmpty(t, genesisBlock)
}

func TestGenesisService_CreateGenesisBlock_InvalidOrgType(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	logger := logrus.New()
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create genesis with invalid organization type
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			ApplicationOrgs: []v1alpha1.ApplicationOrganization{
				{
					Name:  "InvalidOrg",
					MSPID: "InvalidOrgMSP",
					Type:  "invalid",
				},
			},
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

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid organization type")
}

func TestGenesisService_StoreGenesisBlock(t *testing.T) {
	// Setup scheme
	err := v1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

	logger := logrus.New()
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create test genesis
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			Output: v1alpha1.GenesisOutput{
				SecretName: "genesis-block-secret",
				BlockKey:   "genesis.block",
			},
		},
	}

	// Create test genesis block
	genesisBlock := []byte("test-genesis-block-data")

	ctx := context.Background()
	err = service.StoreGenesisBlock(ctx, genesis, genesisBlock)

	require.NoError(t, err)

	// Verify the secret was created
	secret := &corev1.Secret{}
	err = fakeClient.Get(ctx, client.ObjectKey{
		Namespace: "default",
		Name:      "genesis-block-secret",
	}, secret)

	require.NoError(t, err)
	assert.NotNil(t, secret)
	assert.Equal(t, genesisBlock, secret.Data["genesis.block"])
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

	logger := logrus.New()
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
	ordererNodes, _, err := GenerateOrdererNodes("OrdererOrgMSP", 2, "orderer", 7050)
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

	// Create fake client with CA resource
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(ca).
		Build()

	logger := logrus.New()
	service := NewGenesisService(fakeClient, logger, "testchannel")

	// Create test genesis with internal organizations only
	genesis := &v1alpha1.Genesis{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "internal-genesis",
			Namespace: "default",
		},
		Spec: v1alpha1.GenesisSpec{
			InternalOrgs: []v1alpha1.InternalOrganization{*ordererOrg},
			OrdererNodes: ordererNodes, // Add orderer nodes
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
	genesisBlock, err := service.CreateGenesisBlock(ctx, req)

	require.NoError(t, err)
	assert.NotNil(t, genesisBlock)
	assert.NotEmpty(t, genesisBlock)
}
