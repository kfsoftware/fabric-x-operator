package certs

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/hyperledger/fabric-ca/api"
	"github.com/hyperledger/fabric-ca/lib"
	"github.com/hyperledger/fabric-ca/lib/tls"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Key represents a cryptographic key
type Key interface {

	// Bytes converts this key to its byte representation,
	// if this operation is allowed.
	Bytes() ([]byte, error)

	// SKI returns the subject key identifier of this key.
	SKI() []byte

	// Symmetric returns true if this key is a symmetric key,
	// false is this key is asymmetric
	Symmetric() bool

	// Private returns true if this key is a private key,
	// false otherwise.
	Private() bool

	// PublicKey returns the corresponding public key part of an asymmetric public/private key pair.
	// This method returns an error in symmetric key schemes.
	PublicKey() (Key, error)
}

type CertKey struct {
	Cert       *x509.Certificate
	PrivateKey *ecdsa.PrivateKey
}

// Bytes returns the DER-encoded private key bytes.
func (ck *CertKey) Bytes() ([]byte, error) {
	if ck.PrivateKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}
	return x509.MarshalECPrivateKey(ck.PrivateKey)
}

// SKI returns the subject key identifier of the certificate.
func (ck *CertKey) SKI() []byte {
	if ck.Cert == nil {
		return nil
	}
	for _, ext := range ck.Cert.Extensions {
		// OID for Subject Key Identifier: 2.5.29.14
		if ext.Id.Equal([]int{2, 5, 29, 14}) {
			return ext.Value
		}
	}

	ski, err := utils.ComputeSKI(ck.Cert)
	if err != nil {
		return nil
	}
	return ski
}

// Symmetric returns false, as this is an asymmetric key.
func (ck *CertKey) Symmetric() bool {
	return false
}

// Private returns true, as this is a private key.
func (ck *CertKey) Private() bool {
	return true
}

// PublicKey returns a new CertKey with only the public key (certificate).
func (ck *CertKey) PublicKey() (Key, error) {
	if ck.Cert == nil {
		return nil, fmt.Errorf("certificate is nil")
	}
	return &CertKey{
		Cert:       ck.Cert,
		PrivateKey: nil,
	}, nil
}

type FabricPem struct {
	Pem string `yaml:"pem"`
}
type FabricMultiplePem struct {
	Pem []string `yaml:"pem"`
}
type FabricConfigUser struct {
	Key  FabricPem `yaml:"key"`
	Cert FabricPem `yaml:"cert"`
}
type FabricHttpOptions struct {
	Verify bool `yaml:"verify"`
}
type FabricCryptoStore struct {
	Path string `yaml:"path"`
}
type FabricCredentialStore struct {
	Path        string            `yaml:"path"`
	CryptoStore FabricCryptoStore `yaml:"cryptoStore"`
}
type FabricConfigOrg struct {
	Mspid                  string                      `yaml:"mspid"`
	CryptoPath             string                      `yaml:"cryptoPath"`
	Users                  map[string]FabricConfigUser `yaml:"users,omitempty"`
	CredentialStore        FabricCredentialStore       `yaml:"credentialStore,omitempty"`
	CertificateAuthorities []string                    `yaml:"certificateAuthorities"`
}
type FabricRegistrar struct {
	EnrollID     string `yaml:"enrollId"`
	EnrollSecret string `yaml:"enrollSecret"`
}
type FabricConfigCA struct {
	URL         string            `yaml:"url"`
	CaName      string            `yaml:"caName"`
	TLSCACerts  FabricMultiplePem `yaml:"tlsCACerts"`
	Registrar   FabricRegistrar   `yaml:"registrar"`
	HTTPOptions FabricHttpOptions `yaml:"httpOptions"`
}
type FabricConfigTimeoutParams struct {
	Endorser string `yaml:"endorser"`
}
type FabricConfigTimeout struct {
	Peer FabricConfigTimeoutParams `yaml:"peer"`
}
type FabricConfigConnection struct {
	Timeout FabricConfigTimeout `yaml:"timeout"`
}
type FabricConfigClient struct {
	Organization    string                 `yaml:"organization"`
	CredentialStore FabricCredentialStore  `yaml:"credentialStore,omitempty"`
	Connection      FabricConfigConnection `yaml:"connection"`
}
type FabricConfig struct {
	Name                   string                     `yaml:"name"`
	Version                string                     `yaml:"version"`
	Client                 FabricConfigClient         `yaml:"client"`
	Organizations          map[string]FabricConfigOrg `yaml:"organizations"`
	CertificateAuthorities map[string]FabricConfigCA  `yaml:"certificateAuthorities"`
}

type FabricCAParams struct {
	TLSCert      string
	URL          string
	Name         string
	MSPID        string
	EnrollID     string
	EnrollSecret string
}

type EnrollUserRequest struct {
	TLSCert    string
	URL        string
	Name       string
	MSPID      string
	User       string
	Secret     string
	Hosts      []string
	CN         string
	Profile    string
	Attributes []*api.AttributeRequest
}
type ReenrollUserRequest struct {
	EnrollID   string
	TLSCert    string
	URL        string
	Name       string
	MSPID      string
	Hosts      []string
	CN         string
	Profile    string
	Attributes []*api.AttributeRequest
}
type GetCAInfoRequest struct {
	TLSCert string
	URL     string
	Name    string
	MSPID   string
}
type RevokeUserRequest struct {
	TLSCert           string
	URL               string
	Name              string
	MSPID             string
	EnrollID          string
	EnrollSecret      string
	RevocationRequest *api.RevocationRequest
}

func RevokeUser(ctx context.Context, k8sClient client.Client, params RevokeUserRequest) error {
	log := logf.FromContext(ctx)
	caClient, err := GetClient(FabricCAParams{
		TLSCert:      params.TLSCert,
		URL:          params.URL,
		Name:         params.Name,
		MSPID:        params.MSPID,
		EnrollID:     params.EnrollID,
		EnrollSecret: params.EnrollSecret,
	})
	if err != nil {
		return err
	}
	myIdentity, err := caClient.LoadMyIdentity()
	if err != nil {
		return err
	}
	result, err := myIdentity.Revoke(params.RevocationRequest)
	if err != nil {
		return err
	}
	log.Info("Revoked user", "result", result.RevokedCerts)
	return nil
}

type RegisterUserRequest struct {
	TLSCert      string
	URL          string
	Name         string
	MSPID        string
	EnrollID     string
	EnrollSecret string
	User         string
	Secret       string
	Type         string
	Attributes   []api.Attribute
}

func RegisterUser(ctx context.Context, k8sClient client.Client, params RegisterUserRequest) (string, error) {
	log := logf.FromContext(ctx)
	caClient, err := GetClient(FabricCAParams{
		TLSCert:      params.TLSCert,
		URL:          params.URL,
		Name:         params.Name,
		MSPID:        params.MSPID,
		EnrollID:     params.EnrollID,
		EnrollSecret: params.EnrollSecret,
	})
	if err != nil {
		return "", err
	}
	enrollResponse, err := caClient.Enroll(&api.EnrollmentRequest{
		Name:     params.EnrollID,
		Secret:   params.EnrollSecret,
		CAName:   params.Name,
		AttrReqs: []*api.AttributeRequest{},
		Type:     params.Type,
	})
	if err != nil {
		log.Error(err, "Failed to register user")
		return "", errors.Wrap(err, "Failed to register user")
	}
	secret, err := enrollResponse.Identity.Register(&api.RegistrationRequest{
		Name:           params.User,
		Type:           params.Type,
		MaxEnrollments: -1,
		Affiliation:    "",
		Attributes:     params.Attributes,
		CAName:         params.Name,
		Secret:         params.Secret,
	})
	if err != nil {
		return "", err
	}
	return secret.Secret, nil
}

func GetCAInfo(params GetCAInfoRequest) (*lib.GetCAInfoResponse, error) {
	caClient, err := GetClient(FabricCAParams{
		TLSCert: params.TLSCert,
		URL:     params.URL,
		Name:    params.Name,
		MSPID:   params.MSPID,
	})
	if err != nil {
		return nil, err
	}
	caInfo, err := caClient.GetCAInfo(&api.GetCAInfoRequest{})
	if err != nil {
		return nil, err
	}
	return caInfo, nil
}

func readKey(client *lib.Client) (*ecdsa.PrivateKey, error) {
	keystoreDir := filepath.Join(client.HomeDir, "msp", "keystore")
	files, err := ioutil.ReadDir(keystoreDir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("no key found in keystore")
	}
	if len(files) > 1 {
		return nil, errors.New("multiple keys found in keystore")
	}
	keyPath := filepath.Join(keystoreDir, files[0].Name())
	keyBytes, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read key file %s", keyPath)
	}
	ecdsaKey, err := utils.ParseECDSAPrivateKey(keyBytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse key file %s", keyPath)
	}
	return ecdsaKey, nil
}
func EnrollUser(ctx context.Context, params EnrollUserRequest) (*x509.Certificate, *ecdsa.PrivateKey, *x509.Certificate, error) {
	log := logf.FromContext(ctx)
	caClient, err := GetClient(FabricCAParams{
		TLSCert: params.TLSCert,
		URL:     params.URL,
		Name:    params.Name,
		MSPID:   params.MSPID,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	enrollmentRequest := &api.EnrollmentRequest{
		Name:     params.User,
		Secret:   params.Secret,
		CAName:   params.Name,
		AttrReqs: params.Attributes,
		Label:    "",
		Type:     "x509",
		CSR: &api.CSRInfo{
			Hosts: params.Hosts,
			CN:    params.CN,
		},
	}
	log.Info("Enrollment request", "request", enrollmentRequest)
	enrollResponse, err := caClient.Enroll(enrollmentRequest)
	if err != nil {
		return nil, nil, nil, err
	}
	userCrt := enrollResponse.Identity.GetECert().GetX509Cert()

	info, err := caClient.GetCAInfo(&api.GetCAInfoRequest{
		CAName: params.Name,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	rootCrt, err := utils.ParseX509Certificate(info.CAChain)
	if err != nil {
		return nil, nil, nil, err
	}
	userKey, err := readKey(caClient)
	if err != nil {
		return nil, nil, nil, err
	}
	return userCrt, userKey, rootCrt, nil
}

type GetUserRequest struct {
	TLSCert      string
	URL          string
	Name         string
	MSPID        string
	EnrollID     string
	EnrollSecret string
	User         string
}

func GetClient(ca FabricCAParams) (*lib.Client, error) {
	// create temporary directory
	caHomeDir, err := ioutil.TempDir("", "fabric-ca-client")
	if err != nil {
		return nil, nil
	}
	// create temporary file
	caCertFile, err := ioutil.TempFile("", "ca-cert")
	if err != nil {
		return nil, nil
	}
	// write ca cert to file
	_, err = caCertFile.Write([]byte(ca.TLSCert))
	if err != nil {
		return nil, nil
	}
	client := &lib.Client{
		HomeDir: caHomeDir,
		Config: &lib.ClientConfig{
			URL: ca.URL,
			TLS: tls.ClientTLSConfig{
				Enabled:   true,
				CertFiles: []string{caCertFile.Name()},
			},
		},
	}
	err = client.Init()
	if err != nil {
		return nil, err
	}
	return client, err
}

// OrdererGroupCertificateRequest represents a certificate request for an OrdererGroup component
type OrdererGroupCertificateRequest struct {
	// Component name (consenter, batcher, assembler, router)
	ComponentName string

	// Component type for certificate generation
	ComponentType string

	// OrdererGroup namespace
	Namespace string

	// OrdererGroup name
	OrdererGroupName string

	// Certificate configuration
	CertConfig *CertificateConfig

	// Enrollment configuration
	EnrollmentConfig *EnrollmentConfig

	// Certificate types to generate (sign, tls)
	CertTypes []string

	// Enrollment ID
	EnrollID string

	// Enrollment secret
	EnrollSecret string
}

// CertificateConfig represents the certificate configuration from the API
type CertificateConfig struct {
	CAHost       string       `json:"cahost,omitempty"`
	CAName       string       `json:"caname,omitempty"`
	CAPort       int32        `json:"caport,omitempty"`
	CATLS        *CATLSConfig `json:"catls,omitempty"`
	EnrollID     string       `json:"enrollid,omitempty"`
	EnrollSecret string       `json:"enrollsecret,omitempty"`
	MSPID        string       `json:"mspid,omitempty"`
}

// CATLSConfig represents CA TLS configuration
type CATLSConfig struct {
	CACert    string     `json:"cacert,omitempty"`
	SecretRef *SecretRef `json:"secretRef,omitempty"`
}

// SecretRef represents a secret reference
type SecretRef struct {
	Name      string `json:"name"`
	Key       string `json:"key"`
	Namespace string `json:"namespace,omitempty"`
}

// EnrollmentConfig represents enrollment configuration
type EnrollmentConfig struct {
	Sign *CertificateConfig `json:"sign,omitempty"`
	TLS  *CertificateConfig `json:"tls,omitempty"`
}

// ComponentCertificateData represents certificate data for a component
type ComponentCertificateData struct {
	// Component name
	ComponentName string

	// Certificate type (sign, tls)
	CertType string

	// Certificate data
	Cert []byte
	Key  []byte

	// CA certificate
	CACert []byte
}

// CreateSignCertificate checks if sign certificate secret exists and generates it if missing
func CreateSignCertificate(ctx context.Context, k8sClient client.Client, request OrdererGroupCertificateRequest) (*ComponentCertificateData, error) {
	log := logf.FromContext(ctx)

	// Check if sign certificate secret already exists
	secretName := fmt.Sprintf("%s-sign-cert", request.ComponentName)
	existingSecret := &corev1.Secret{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      secretName,
		Namespace: request.Namespace,
	}, existingSecret)

	if err == nil {
		log.Info("Sign certificate secret already exists, skipping generation",
			"component", request.ComponentName,
			"secret", secretName)
		return nil, nil
	}

	if !k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check existing sign certificate secret %s: %w", secretName, err)
	}

	log.Info("Sign certificate secret not found, generating new sign certificate",
		"component", request.ComponentName,
		"secret", secretName)

	certData, err := provisionComponentCertificateWithClient(ctx, k8sClient, request, "sign")
	if err != nil {
		return nil, fmt.Errorf("failed to provision sign certificate for %s: %w", request.ComponentName, err)
	}

	log.Info("Successfully generated sign certificate",
		"component", request.ComponentName)

	return certData, nil
}

// CreateTLSCertificate checks if TLS certificate secret exists and generates it if missing
func CreateTLSCertificate(ctx context.Context, k8sClient client.Client, request OrdererGroupCertificateRequest) (*ComponentCertificateData, error) {
	log := logf.FromContext(ctx)

	// Check if TLS certificate secret already exists
	secretName := fmt.Sprintf("%s-tls-cert", request.ComponentName)
	existingSecret := &corev1.Secret{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Name:      secretName,
		Namespace: request.Namespace,
	}, existingSecret)

	if err == nil {
		log.Info("TLS certificate secret already exists, skipping generation",
			"component", request.ComponentName,
			"secret", secretName)
		return nil, nil
	}

	if !k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check existing TLS certificate secret %s: %w", secretName, err)
	}

	log.Info("TLS certificate secret not found, generating new TLS certificate",
		"component", request.ComponentName,
		"secret", secretName)

	certData, err := provisionComponentCertificateWithClient(ctx, k8sClient, request, "tls")
	if err != nil {
		return nil, fmt.Errorf("failed to provision TLS certificate for %s: %w", request.ComponentName, err)
	}

	log.Info("Successfully generated TLS certificate",
		"component", request.ComponentName)

	return certData, nil
}

// ProvisionSignCertificate provisions only the sign certificate for a component
func ProvisionSignCertificate(ctx context.Context, k8sClient client.Client, request OrdererGroupCertificateRequest) (*ComponentCertificateData, error) {
	log := logf.FromContext(ctx)

	log.Info("Provisioning sign certificate",
		"component", request.ComponentName,
		"componentType", request.ComponentType)

	certData, err := provisionComponentCertificateWithClient(ctx, k8sClient, request, "sign")
	if err != nil {
		return nil, fmt.Errorf("failed to provision sign certificate for %s: %w", request.ComponentName, err)
	}

	log.Info("Successfully provisioned sign certificate",
		"component", request.ComponentName)

	return certData, nil
}

// ProvisionTLSCertificate provisions only the TLS certificate for a component
func ProvisionTLSCertificate(ctx context.Context, k8sClient client.Client, request OrdererGroupCertificateRequest) (*ComponentCertificateData, error) {
	log := logf.FromContext(ctx)

	log.Info("Provisioning TLS certificate",
		"component", request.ComponentName,
		"componentType", request.ComponentType)

	certData, err := provisionComponentCertificateWithClient(ctx, k8sClient, request, "tls")
	if err != nil {
		return nil, fmt.Errorf("failed to provision TLS certificate for %s: %w", request.ComponentName, err)
	}

	log.Info("Successfully provisioned TLS certificate",
		"component", request.ComponentName)

	return certData, nil
}

// ProvisionSpecificCertificate provisions a specific certificate type for a component
func ProvisionSpecificCertificate(ctx context.Context, k8sClient client.Client, request OrdererGroupCertificateRequest, certType string) (*ComponentCertificateData, error) {
	log := logf.FromContext(ctx)

	log.Info("Provisioning specific certificate",
		"component", request.ComponentName,
		"componentType", request.ComponentType,
		"certType", certType)

	certData, err := provisionComponentCertificateWithClient(ctx, k8sClient, request, certType)
	if err != nil {
		return nil, fmt.Errorf("failed to provision %s certificate for %s: %w", certType, request.ComponentName, err)
	}

	log.Info("Successfully provisioned specific certificate",
		"component", request.ComponentName,
		"certType", certType)

	return certData, nil
}

// ProvisionOrdererGroupCertificates provisions certificates for all components in an OrdererGroup
func ProvisionOrdererGroupCertificates(ctx context.Context, request OrdererGroupCertificateRequest) ([]ComponentCertificateData, error) {
	var certificates []ComponentCertificateData

	// Determine which certificate types to generate
	certTypes := request.CertTypes
	if len(certTypes) == 0 {
		certTypes = []string{"sign", "tls"}
	}

	// Generate certificates for each type
	for _, certType := range certTypes {
		certData, err := provisionComponentCertificate(ctx, request, certType)
		if err != nil {
			return nil, fmt.Errorf("failed to provision %s certificate for %s: %w", certType, request.ComponentName, err)
		}
		certificates = append(certificates, *certData)
	}

	return certificates, nil
}

// ProvisionOrdererGroupCertificatesWithClient provisions certificates for all components in an OrdererGroup with client context
func ProvisionOrdererGroupCertificatesWithClient(ctx context.Context, k8sClient client.Client, request OrdererGroupCertificateRequest) ([]ComponentCertificateData, error) {
	var certificates []ComponentCertificateData

	// Determine which certificate types to generate
	certTypes := request.CertTypes
	if len(certTypes) == 0 {
		certTypes = []string{"sign", "tls"}
	}

	// Generate certificates for each type
	for _, certType := range certTypes {
		certData, err := provisionComponentCertificateWithClient(ctx, k8sClient, request, certType)
		if err != nil {
			return nil, fmt.Errorf("failed to provision %s certificate for %s: %w", certType, request.ComponentName, err)
		}
		certificates = append(certificates, *certData)
	}
	return certificates, nil
}

// provisionComponentCertificate provisions a certificate for a specific component and type
func provisionComponentCertificate(ctx context.Context, request OrdererGroupCertificateRequest, certType string) (*ComponentCertificateData, error) {
	log := logf.FromContext(ctx)
	// Determine which certificate configuration to use
	var certConfig *CertificateConfig
	var enrollmentConfig *EnrollmentConfig

	if request.CertConfig != nil {
		// Use component-specific certificate configuration
		certConfig = request.CertConfig
	} else if request.EnrollmentConfig != nil {
		// Use global enrollment configuration
		enrollmentConfig = request.EnrollmentConfig
		if certType == "sign" && enrollmentConfig.Sign != nil {
			certConfig = enrollmentConfig.Sign
		} else if certType == "tls" && enrollmentConfig.TLS != nil {
			certConfig = enrollmentConfig.TLS
		}
	}

	if certConfig == nil {
		return nil, fmt.Errorf("no certificate configuration found for type %s", certType)
	}

	// Get CA certificate
	caCert, err := getCACertificate(ctx, nil, certConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get CA certificate: %w", err)
	}

	// Generate component-specific enrollment parameters
	hosts := generateComponentHosts(request.ComponentName, request.OrdererGroupName, request.Namespace)
	cn := generateComponentCN(request.ComponentName, request.OrdererGroupName, certType)

	// Create enrollment request
	enrollRequest := EnrollUserRequest{
		TLSCert:    caCert,
		URL:        fmt.Sprintf("https://%s:%d", certConfig.CAHost, certConfig.CAPort),
		Name:       certConfig.CAName,
		MSPID:      certConfig.MSPID,
		User:       request.EnrollID,
		Secret:     request.EnrollSecret,
		Hosts:      hosts,
		CN:         cn,
		Attributes: []*api.AttributeRequest{},
	}

	// Enroll the component
	userCert, userKey, rootCert, err := EnrollUser(ctx, enrollRequest)
	if err != nil {
		log.Error(err, "Failed to enroll component", "component", request.ComponentName, "enrollID", request.EnrollID)
		return nil, fmt.Errorf("failed to enroll component %s (enrollID: %s): %w", request.ComponentName, enrollRequest.User, err)
	}

	// Convert certificates to PEM format
	userCertPEM := utils.EncodeX509Certificate(userCert)
	userKeyPEM, err := utils.EncodePrivateKey(userKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encode private key: %w", err)
	}
	rootCertPEM := utils.EncodeX509Certificate(rootCert)

	return &ComponentCertificateData{
		ComponentName: request.ComponentName,
		CertType:      certType,
		Cert:          userCertPEM,
		Key:           userKeyPEM,
		CACert:        rootCertPEM,
	}, nil
}

// provisionComponentCertificateWithClient provisions a certificate for a specific component and type with client context
func provisionComponentCertificateWithClient(ctx context.Context, k8sClient client.Client, request OrdererGroupCertificateRequest, certType string) (*ComponentCertificateData, error) {
	log := logf.FromContext(ctx)
	// Determine which certificate configuration to use
	var certConfig *CertificateConfig
	var enrollmentConfig *EnrollmentConfig

	if request.CertConfig != nil {
		// Use component-specific certificate configuration
		certConfig = request.CertConfig
	} else if request.EnrollmentConfig != nil {
		// Use global enrollment configuration
		enrollmentConfig = request.EnrollmentConfig
		if certType == "sign" && enrollmentConfig.Sign != nil {
			certConfig = enrollmentConfig.Sign
		} else if certType == "tls" && enrollmentConfig.TLS != nil {
			certConfig = enrollmentConfig.TLS
		}
	}

	if certConfig == nil {
		return nil, fmt.Errorf("no certificate configuration found for type %s", certType)
	}

	// Get CA certificate
	caCert, err := getCACertificate(ctx, k8sClient, certConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get CA certificate: %w", err)
	}

	// Generate component-specific enrollment parameters
	enrollID := request.EnrollID
	secret := request.EnrollSecret
	hosts := generateComponentHosts(request.ComponentName, request.OrdererGroupName, request.Namespace)
	cn := generateComponentCN(request.ComponentName, request.OrdererGroupName, certType)

	// Create enrollment request
	enrollRequest := EnrollUserRequest{
		TLSCert:    caCert,
		URL:        fmt.Sprintf("https://%s:%d", certConfig.CAHost, certConfig.CAPort),
		Name:       certConfig.CAName,
		MSPID:      certConfig.MSPID, // Default MSP ID for orderer components
		User:       enrollID,
		Secret:     secret,
		Hosts:      hosts,
		CN:         cn,
		Attributes: []*api.AttributeRequest{},
	}

	// Enroll the component
	userCert, userKey, rootCert, err := EnrollUser(ctx, enrollRequest)
	if err != nil {
		log.Error(err, "Failed to enroll component", "component", request.ComponentName, "enrollID", request.EnrollID)
		return nil, fmt.Errorf("failed to enroll component %s (enrollID: %s): %w", request.ComponentName, enrollID, err)
	}

	// Convert certificates to PEM format
	userCertPEM := utils.EncodeX509Certificate(userCert)
	userKeyPEM, err := utils.EncodePrivateKey(userKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encode private key: %w", err)
	}
	rootCertPEM := utils.EncodeX509Certificate(rootCert)

	return &ComponentCertificateData{
		ComponentName: request.ComponentName,
		CertType:      certType,
		Cert:          userCertPEM,
		Key:           userKeyPEM,
		CACert:        rootCertPEM,
	}, nil
}

// getCACertificate retrieves the CA certificate from the configuration
func getCACertificate(ctx context.Context, k8sClient client.Client, certConfig *CertificateConfig) (string, error) {
	if certConfig.CATLS == nil {
		return "", fmt.Errorf("CA TLS configuration is required")
	}

	// If CA certificate is directly provided
	if certConfig.CATLS.CACert != "" {
		return certConfig.CATLS.CACert, nil
	}

	// If CA certificate is in a secret
	if certConfig.CATLS.SecretRef != nil {
		if k8sClient == nil {
			return "", fmt.Errorf("Kubernetes client is required to read CA certificate from secret")
		}

		secretRef := certConfig.CATLS.SecretRef

		// Determine namespace
		namespace := secretRef.Namespace
		if namespace == "" {
			namespace = "default"
		}

		// Get the secret
		secret := &corev1.Secret{}
		err := k8sClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      secretRef.Name,
		}, secret)
		if err != nil {
			return "", fmt.Errorf("failed to get CA certificate secret %s/%s: %w", namespace, secretRef.Name, err)
		}

		// Extract the certificate from the secret
		certData, exists := secret.Data[secretRef.Key]
		if !exists {
			return "", fmt.Errorf("CA certificate key '%s' not found in secret %s/%s", secretRef.Key, namespace, secretRef.Name)
		}

		return string(certData), nil
	}

	return "", fmt.Errorf("no CA certificate found in configuration")
}

// generateComponentHosts generates host names for a component
func generateComponentHosts(componentName, ordererGroupName, namespace string) []string {
	return []string{
		fmt.Sprintf("%s-%s", componentName, ordererGroupName),
		fmt.Sprintf("%s-%s.%s", componentName, ordererGroupName, namespace),
		fmt.Sprintf("%s-%s.%s.svc.cluster.local", componentName, ordererGroupName, namespace),
	}
}

// generateComponentCN generates a common name for a component
func generateComponentCN(componentName, ordererGroupName, certType string) string {
	return fmt.Sprintf("%s-%s-%s", componentName, ordererGroupName, certType)
}
