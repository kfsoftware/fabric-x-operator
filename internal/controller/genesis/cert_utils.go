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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/kfsoftware/fabric-x-operator/api/v1alpha1"
)

// CertificateBundle represents a collection of certificates for an organization
type CertificateBundle struct {
	CA        []byte
	TLS       []byte
	Admin     []byte
	Peer      []byte
	Orderer   []byte
	Consenter []byte
	Router    []byte
	Batcher   []byte
	Assembler []byte
}

// OrganizationConfig represents configuration for generating organization certificates
type OrganizationConfig struct {
	Name               string
	MSPID              string
	Country            string
	State              string
	Locality           string
	Organization       string
	OrganizationalUnit string
	CommonName         string
	ValidDays          int
}

// DefaultOrganizationConfig returns a default organization configuration
func DefaultOrganizationConfig(name, mspID string) *OrganizationConfig {
	return &OrganizationConfig{
		Name:               name,
		MSPID:              mspID,
		Country:            "US",
		State:              "California",
		Locality:           "San Francisco",
		Organization:       name,
		OrganizationalUnit: "Hyperledger Fabric",
		CommonName:         name,
		ValidDays:          365,
	}
}

// GenerateOrganizationCertificates generates a complete set of certificates for an organization
func GenerateOrganizationCertificates(config *OrganizationConfig) (*CertificateBundle, error) {
	// Generate root private key
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// Generate TLS root private key
	tlsRootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// Generate CA certificate
	caCert, err := generateCACertificate(config, rootKey)
	if err != nil {
		return nil, err
	}

	// Generate TLS CA certificate
	tlsCACert, err := generateTLSCACertificate(config, tlsRootKey)
	if err != nil {
		return nil, err
	}

	// Generate admin certificate
	adminCert, err := generateAdminCertificate(config, rootKey, caCert)
	if err != nil {
		return nil, err
	}

	// Generate peer certificate
	peerCert, err := generatePeerCertificate(config, rootKey, caCert)
	if err != nil {
		return nil, err
	}

	// Generate orderer certificate
	ordererCert, err := generateOrdererCertificate(config, rootKey, caCert)
	if err != nil {
		return nil, err
	}

	// Generate consenter certificate
	consenterCert, err := generateConsenterCertificate(config, rootKey, caCert)
	if err != nil {
		return nil, err
	}

	// Generate router certificate
	routerCert, err := generateRouterCertificate(config, tlsRootKey, tlsCACert)
	if err != nil {
		return nil, err
	}

	// Generate batcher certificate
	batcherCert, err := generateBatcherCertificate(config, rootKey, caCert)
	if err != nil {
		return nil, err
	}

	// Generate assembler certificate
	assemblerCert, err := generateAssemblerCertificate(config, tlsRootKey, tlsCACert)
	if err != nil {
		return nil, err
	}

	return &CertificateBundle{
		CA:        caCert,
		TLS:       tlsCACert,
		Admin:     adminCert,
		Peer:      peerCert,
		Orderer:   ordererCert,
		Consenter: consenterCert,
		Router:    routerCert,
		Batcher:   batcherCert,
		Assembler: assemblerCert,
	}, nil
}

// GenerateApplicationOrganization creates an application organization with certificates
func GenerateApplicationOrganization(name, mspID, orgType string) (*v1alpha1.ApplicationOrganization, *CertificateBundle, error) {
	config := DefaultOrganizationConfig(name, mspID)
	certBundle, err := GenerateOrganizationCertificates(config)
	if err != nil {
		return nil, nil, err
	}

	appOrg := &v1alpha1.ApplicationOrganization{
		Name:  name,
		MSPID: mspID,
		SignCACertRef: v1alpha1.SecretKeyNSSelector{
			Name:      name + "-sign-ca-secret",
			Namespace: "default",
			Key:       "ca.crt",
		},
		TLSCACertRef: v1alpha1.SecretKeyNSSelector{
			Name:      name + "-tls-ca-secret",
			Namespace: "default",
			Key:       "ca.crt",
		},
		AdminCertRef: &v1alpha1.SecretKeyNSSelector{
			Name:      name + "-admin-secret",
			Namespace: "default",
			Key:       "admin.crt",
		},
	}

	return appOrg, certBundle, nil
}

// GenerateOrdererOrganization creates an orderer organization with certificates
func GenerateOrdererOrganization(name, mspID string) (*v1alpha1.OrdererOrganization, *CertificateBundle, error) {
	config := DefaultOrganizationConfig(name, mspID)
	certBundle, err := GenerateOrganizationCertificates(config)
	if err != nil {
		return nil, nil, err
	}

	ordererOrg := &v1alpha1.OrdererOrganization{
		Name:  name,
		MSPID: mspID,
		SignCACertRef: v1alpha1.SecretKeyNSSelector{
			Name:      name + "-sign-ca-secret",
			Namespace: "default",
			Key:       "ca.crt",
		},
		TLSCACertRef: v1alpha1.SecretKeyNSSelector{
			Name:      name + "-tls-ca-secret",
			Namespace: "default",
			Key:       "ca.crt",
		},
		AdminCertRef: &v1alpha1.SecretKeyNSSelector{
			Name:      name + "-admin-secret",
			Namespace: "default",
			Key:       "admin.crt",
		},
	}

	return ordererOrg, certBundle, nil
}

// GenerateExternalOrganization creates an external organization with certificates
func GenerateExternalOrganization(name, mspID string) (*v1alpha1.OrdererOrganization, *CertificateBundle, error) {
	config := DefaultOrganizationConfig(name, mspID)
	certBundle, err := GenerateOrganizationCertificates(config)
	if err != nil {
		return nil, nil, err
	}

	externalOrg := &v1alpha1.OrdererOrganization{
		Name:  name,
		MSPID: mspID,
		SignCACertRef: v1alpha1.SecretKeyNSSelector{
			Name:      name + "-sign-ca-secret",
			Namespace: "default",
			Key:       "ca.crt",
		},
		TLSCACertRef: v1alpha1.SecretKeyNSSelector{
			Name:      name + "-tls-ca-secret",
			Namespace: "default",
			Key:       "ca.crt",
		},
		AdminCertRef: &v1alpha1.SecretKeyNSSelector{
			Name:      name + "-admin-secret",
			Namespace: "default",
			Key:       "admin.crt",
		},
	}

	return externalOrg, certBundle, nil
}

// GenerateConsenters creates consenter nodes with certificates
func GenerateConsenters(mspID string, count int, baseHost string, basePort int) ([]v1alpha1.OrdererNode, *CertificateBundle, error) {
	config := DefaultOrganizationConfig("OrdererOrg", mspID)
	certBundle, err := GenerateOrganizationCertificates(config)
	if err != nil {
		return nil, nil, err
	}

	var nodes []v1alpha1.OrdererNode
	for i := 1; i <= count; i++ {
		node := v1alpha1.OrdererNode{
			ID:    i,
			Host:  fmt.Sprintf("%s%d", baseHost, i),
			Port:  basePort + i - 1,
			MSPID: mspID,
			ClientTLSCertRef: v1alpha1.SecretKeyNSSelector{
				Name:      fmt.Sprintf("orderer%d-tls-secret", i),
				Namespace: "default",
				Key:       "tls.crt",
			},
			ServerTLSCertRef: v1alpha1.SecretKeyNSSelector{
				Name:      fmt.Sprintf("orderer%d-tls-secret", i),
				Namespace: "default",
				Key:       "tls.crt",
			},
			IdentityRef: v1alpha1.SecretKeyNSSelector{
				Name:      fmt.Sprintf("orderer%d-identity-secret", i),
				Namespace: "default",
				Key:       "identity.crt",
			},
		}
		nodes = append(nodes, node)
	}

	return nodes, certBundle, nil
}

// generateCACertificate generates a CA certificate
func generateCACertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         config.CommonName + " CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, config.ValidDays),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}

// generateTLSCACertificate generates a TLS CA certificate
func generateTLSCACertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         config.CommonName + " TLS CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, config.ValidDays),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}

// generateAdminCertificate generates an admin certificate
func generateAdminCertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey, caCert []byte) ([]byte, error) {
	// Parse CA certificate
	caBlock, _ := pem.Decode(caCert)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Generate admin private key
	adminKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         "admin@" + config.CommonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, config.ValidDays),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &adminKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}

// generatePeerCertificate generates a peer certificate
func generatePeerCertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey, caCert []byte) ([]byte, error) {
	// Parse CA certificate
	caBlock, _ := pem.Decode(caCert)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Generate peer private key
	peerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(4),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         "peer@" + config.CommonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, config.ValidDays),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &peerKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}

// generateOrdererCertificate generates an orderer certificate
func generateOrdererCertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey, caCert []byte) ([]byte, error) {
	// Parse CA certificate
	caBlock, _ := pem.Decode(caCert)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Generate orderer private key
	ordererKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(5),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         "orderer@" + config.CommonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, config.ValidDays),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &ordererKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}

// generateConsenterCertificate generates a consenter certificate
func generateConsenterCertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey, caCert []byte) ([]byte, error) {
	// Parse CA certificate
	caBlock, _ := pem.Decode(caCert)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Generate consenter private key
	consenterKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(6),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         "consenter@" + config.CommonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, config.ValidDays),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &consenterKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}

// generateRouterCertificate generates a router certificate
func generateRouterCertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey, caCert []byte) ([]byte, error) {
	// Parse CA certificate
	caBlock, _ := pem.Decode(caCert)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Generate router private key
	routerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(7),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         "router@" + config.CommonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, config.ValidDays),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &routerKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}

// generateBatcherCertificate generates a batcher certificate
func generateBatcherCertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey, caCert []byte) ([]byte, error) {
	// Parse CA certificate
	caBlock, _ := pem.Decode(caCert)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Generate batcher private key
	batcherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(8),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         "batcher@" + config.CommonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, config.ValidDays),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &batcherKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}

// generateAssemblerCertificate generates an assembler certificate
func generateAssemblerCertificate(config *OrganizationConfig, privateKey *ecdsa.PrivateKey, caCert []byte) ([]byte, error) {
	// Parse CA certificate
	caBlock, _ := pem.Decode(caCert)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, err
	}

	// Generate assembler private key
	assemblerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(9),
		Subject: pkix.Name{
			Country:            []string{config.Country},
			Province:           []string{config.State},
			Locality:           []string{config.Locality},
			Organization:       []string{config.Organization},
			OrganizationalUnit: []string{config.OrganizationalUnit},
			CommonName:         "assembler@" + config.CommonName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 0, config.ValidDays),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &assemblerKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), nil
}
