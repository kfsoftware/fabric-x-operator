package utils

import (
	"crypto/ecdsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
)

func ParseECDSAPrivateKey(contents []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(contents)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not of ECDSA type")
	}
	return ecdsaKey, nil
}

func ParseX509Certificate(contents []byte) (*x509.Certificate, error) {
	if len(contents) == 0 {
		return nil, errors.New("certificate pem is empty")
	}
	block, _ := pem.Decode(contents)
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return crt, nil
}

func EncodeX509Certificate(crt *x509.Certificate) []byte {
	pemPk := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: crt.Raw,
	})
	return pemPk
}

func EncodePrivateKey(key interface{}) ([]byte, error) {
	signEncodedPK, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	pemPk := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: signEncodedPK,
	})
	return pemPk, nil
}

type subjectPublicKeyInfo struct {
	Algorithm        pkix.AlgorithmIdentifier
	SubjectPublicKey asn1.BitString
}

// ComputeSKI derives an SKI from the certificate's public key in a
// standard manner. This is done by computing the SHA-1 digest of the
// SubjectPublicKeyInfo component of the certificate.
func ComputeSKI(template *x509.Certificate) ([]byte, error) {
	pub := template.PublicKey
	encodedPub, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}

	var subPKI subjectPublicKeyInfo
	_, err = asn1.Unmarshal(encodedPub, &subPKI)
	if err != nil {
		return nil, err
	}

	pubHash := sha1.Sum(subPKI.SubjectPublicKey.Bytes)
	return pubHash[:], nil
}
