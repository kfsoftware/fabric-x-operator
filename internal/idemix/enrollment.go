package idemix

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyperledger/fabric-ca/api"
	"github.com/hyperledger/fabric-ca/lib"
	idemix2 "github.com/hyperledger/fabric-ca/lib/client/credential/idemix"
	"github.com/hyperledger/fabric-ca/lib/tls"
	"github.com/hyperledger/fabric-lib-go/bccsp/factory"
)

// EnrollmentRequest represents a request to enroll and get idemix credentials
type EnrollmentRequest struct {
	// CA URL (e.g., "https://ca.example.com:7054")
	CAURL string

	// CA name
	CAName string

	// Enrollment ID
	EnrollID string

	// Enrollment password/secret
	EnrollSecret string

	// CA TLS certificate (PEM encoded) for TLS verification
	// Path to CA cert file
	CACertPath string

	// Skip TLS verification (not recommended for production)
	SkipTLSVerify bool

	// MSP Directory to store credentials
	MSPDir string
}

// EnrollmentResponse contains the idemix credential and related information
type EnrollmentResponse struct {
	// SignerConfig contains the complete idemix credential
	SignerConfig *idemix2.SignerConfig

	// Path to the idemix credential directory
	IdemixConfigPath string
}

// Enroll performs idemix enrollment with the Fabric CA using the official client library
func Enroll(req EnrollmentRequest) (*EnrollmentResponse, error) {
	// Create a temporary directory for the MSP if not provided
	mspDir := req.MSPDir
	if mspDir == "" {
		tmpDir, err := os.MkdirTemp("", "idemix-msp-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp MSP directory: %w", err)
		}
		mspDir = tmpDir
	}

	// Ensure MSP directory exists
	if err := os.MkdirAll(mspDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create MSP directory: %w", err)
	}

	// Initialize BCCSP (crypto service provider)
	bccspDir := filepath.Join(mspDir, "keystore")
	if err := os.MkdirAll(bccspDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create BCCSP directory: %w", err)
	}

	opts := &factory.FactoryOpts{
		Default: "SW",
		SW: &factory.SwOpts{
			Hash:     "SHA2",
			Security: 256,
			FileKeystore: &factory.FileKeystoreOpts{
				KeyStorePath: bccspDir,
			},
		},
	}

	if err := factory.InitFactories(opts); err != nil {
		return nil, fmt.Errorf("failed to initialize BCCSP: %w", err)
	}

	// Create Fabric CA client configuration
	tlsConfig := tls.ClientTLSConfig{
		Enabled: true,
	}

	// Set TLS configuration
	if req.SkipTLSVerify {
		// Skip TLS verification (not recommended for production)
		// Note: fabric-ca client doesn't have a direct "skip verify" option,
		// but we can create an empty cert list which effectively skips verification
		tlsConfig.CertFiles = []string{}
	} else if req.CACertPath != "" {
		tlsConfig.CertFiles = []string{req.CACertPath}
	}

	clientConfig := &lib.ClientConfig{
		URL:    req.CAURL,
		TLS:    tlsConfig,
		MSPDir: mspDir,
		CSP:    opts,
		Idemix: api.Idemix{
			Curve: "gurvy.Bn254", // Match the curve used by the CA server
		},
	}

	// Create client
	client := &lib.Client{
		Config:  clientConfig,
		HomeDir: mspDir,
	}

	// Initialize client
	if err := client.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize CA client: %w", err)
	}

	// Create enrollment request
	enrollReq := &api.EnrollmentRequest{
		Name:   req.EnrollID,
		Secret: req.EnrollSecret,
		CAName: req.CAName,
		Type:   "idemix",
	}

	// Perform enrollment
	enrollResp, err := client.Enroll(enrollReq)
	if err != nil {
		return nil, fmt.Errorf("failed to enroll: %w", err)
	}

	// Get the identity
	identity := enrollResp.Identity

	// Get idemix credential from identity
	idemixCred := identity.GetIdemixCredential()
	if idemixCred == nil {
		return nil, fmt.Errorf("no idemix credential returned from enrollment")
	}

	// Store the credential
	idemixConfigPath := filepath.Join(mspDir, "user")
	if err := idemixCred.Store(); err != nil {
		return nil, fmt.Errorf("failed to store idemix credential: %w", err)
	}

	// Load the signer config
	signerConfigPath := filepath.Join(idemixConfigPath, "SignerConfig")
	signerConfigBytes, err := os.ReadFile(signerConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read signer config: %w", err)
	}

	var signerConfig idemix2.SignerConfig
	if err := json.Unmarshal(signerConfigBytes, &signerConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal signer config: %w", err)
	}

	return &EnrollmentResponse{
		SignerConfig:     &signerConfig,
		IdemixConfigPath: idemixConfigPath,
	}, nil
}

// GetIdemixMSPConfig returns the idemix MSP configuration files structure
// This matches the structure expected by Fabric applications
func GetIdemixMSPConfig(signerConfig *idemix2.SignerConfig, issuerPubKey []byte, revocationPubKey []byte) (map[string][]byte, error) {
	if signerConfig == nil {
		return nil, fmt.Errorf("signer config is nil")
	}

	// Marshal signer config
	signerConfigBytes, err := json.Marshal(signerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signer config: %w", err)
	}

	files := map[string][]byte{
		// User directory contains the signer config
		"user/SignerConfig": signerConfigBytes,

		// MSP directory contains issuer public key
		"msp/IssuerPublicKey": issuerPubKey,

		// MSP directory contains revocation public key
		"msp/RevocationPublicKey": revocationPubKey,
	}

	return files, nil
}
