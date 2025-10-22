package enrollment

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/hyperledger/fabric-ca/api"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/certs"
	"github.com/kfsoftware/fabric-x-operator/internal/controller/utils"
)

// X509EnrollmentRequest represents a request to enroll and get X.509 certificates
type X509EnrollmentRequest struct {
	// CA URL (e.g., "https://ca.example.com:7054")
	CAURL string

	// CA name
	CAName string

	// Enrollment ID
	EnrollID string

	// Enrollment password/secret
	EnrollSecret string

	// CA TLS certificate (PEM encoded) for TLS verification
	CATLSCert string

	// MSPID for the organization
	MSPID string

	// Common Name for the certificate
	CN string

	// Subject Alternative Names (hosts)
	Hosts []string

	// Email addresses for certificate SANs
	EmailAddresses []string

	// URIs for certificate SANs
	URIs []string

	// Profile to use for enrollment ("" for sign, "tls" for TLS)
	Profile string

	// Attributes to request in the certificate
	Attributes []*api.AttributeRequest
}

// X509EnrollmentResponse contains the X.509 certificate and related information
type X509EnrollmentResponse struct {
	// Certificate (PEM encoded)
	Certificate []byte

	// Private key (PEM encoded)
	PrivateKey []byte

	// CA Certificate (PEM encoded)
	CACertificate []byte

	// Raw certificate (parsed)
	CertificateRaw *x509.Certificate

	// Raw private key (parsed)
	PrivateKeyRaw *ecdsa.PrivateKey

	// Raw CA certificate (parsed)
	CACertificateRaw *x509.Certificate
}

// EnrollX509 performs X.509 enrollment with the Fabric CA
func EnrollX509(ctx context.Context, req X509EnrollmentRequest) (*X509EnrollmentResponse, error) {
	// Perform enrollment using existing certs package
	userCert, userKey, rootCert, err := certs.EnrollUser(ctx, certs.EnrollUserRequest{
		TLSCert:        req.CATLSCert,
		URL:            req.CAURL,
		Name:           req.CAName,
		MSPID:          req.MSPID,
		User:           req.EnrollID,
		Secret:         req.EnrollSecret,
		Hosts:          req.Hosts,
		CN:             req.CN,
		Profile:        req.Profile,
		Attributes:     req.Attributes,
		EmailAddresses: req.EmailAddresses,
		URIs:           req.URIs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to enroll user: %w", err)
	}

	// Convert to PEM format
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: userCert.Raw})
	keyPEM, err := utils.EncodePrivateKey(userKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encode private key: %w", err)
	}
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootCert.Raw})

	return &X509EnrollmentResponse{
		Certificate:      certPEM,
		PrivateKey:       keyPEM,
		CACertificate:    caCertPEM,
		CertificateRaw:   userCert,
		PrivateKeyRaw:    userKey,
		CACertificateRaw: rootCert,
	}, nil
}

// EnrollX509Pair performs both sign and TLS X.509 enrollments
func EnrollX509Pair(ctx context.Context, signReq, tlsReq X509EnrollmentRequest) (*X509EnrollmentResponse, *X509EnrollmentResponse, error) {
	// Perform sign certificate enrollment
	signResp, err := EnrollX509(ctx, signReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enroll sign certificate: %w", err)
	}

	// Perform TLS certificate enrollment if requested
	var tlsResp *X509EnrollmentResponse
	if tlsReq.CAURL != "" {
		tlsResp, err = EnrollX509(ctx, tlsReq)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to enroll TLS certificate: %w", err)
		}
	}

	return signResp, tlsResp, nil
}
