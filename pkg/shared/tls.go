package shared

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"time"
)

// TLSConfigOptions holds configuration options for TLS certificate generation
type TLSConfigOptions struct {
	Organization string
	DNSNames     []string
	IPAddresses  []net.IP
}

// GenerateTLSConfig generates a TLS configuration with a self-signed certificate
func GenerateTLSConfig(opts TLSConfigOptions) (*tls.Config, error) {
	// Generate private key
	key, err := rsa.GenerateKey(rand.Reader, TLSKeyBits)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Set default values
	if opts.Organization == "" {
		opts.Organization = "QUIC Server"
	}
	if len(opts.IPAddresses) == 0 {
		opts.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1)}
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{opts.Organization},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(CertValidityPeriod),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  opts.IPAddresses,
		DNSNames:     opts.DNSNames,
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Create TLS certificate
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h3"},
	}, nil
}