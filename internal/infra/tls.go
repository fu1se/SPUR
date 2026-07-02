// Package infra holds low-level wiring (TLS, config, logging) used by the
// composition root in cmd/app. It is the outermost layer: it may import
// adapter, usecase and domain, but nothing may import infra except cmd/app.
package infra

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"time"
)

// SelfSignedServerTLSConfig generates an ephemeral self-signed certificate
// for the control-plane QUIC listener.
//
// This is a deliberate interim simplification (see CLAUDE.md, "Требования
// окружения для сборки"): proper certificate verification — pinning the
// client to the server's known public key instead of trusting anything —
// is scheduled for Phase 7 and must land before this is exposed on an
// untrusted network.
func SelfSignedServerTLSConfig(alpn string) (*tls.Config, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("infra: generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("infra: generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "localizator control-plane"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * 365 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("infra: create certificate: %w", err)
	}

	cert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{alpn},
	}, nil
}

// InsecureClientTLSConfig trusts any server certificate. See the warning on
// SelfSignedServerTLSConfig — this must be replaced by real verification in
// Phase 7.
func InsecureClientTLSConfig(alpn string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // interim, see Phase 7 in CLAUDE.md
		NextProtos:         []string{alpn},
	}
}
