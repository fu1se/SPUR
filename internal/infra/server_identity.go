package infra

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// DefaultServerCertPath is where LoadOrCreateServerTLSConfig looks by
// default.
func DefaultServerCertPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("infra: user config dir: %w", err)
	}
	return filepath.Join(dir, "localizator", "server.pem"), nil
}

// LoadOrCreateServerTLSConfig persists a self-signed control-plane
// certificate (and its key) across server restarts at path.
//
// This exists for TOFUClientTLSConfig's benefit: a client pins whatever
// certificate a server presents on first connect and expects it
// unchanged afterwards. SelfSignedServerTLSConfig's ephemeral,
// regenerate-every-run certificate would make every restart look like a
// server impersonation to a client that already pinned it — the whole
// point of pinning is defeated if the pinned value never stays valid.
func LoadOrCreateServerTLSConfig(path, alpn string) (*tls.Config, error) {
	cert, err := loadServerCertificate(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cert, err = createAndSaveServerCertificate(path)
		if err != nil {
			return nil, err
		}
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{alpn},
	}, nil
}

func loadServerCertificate(path string) (tls.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return tls.Certificate{}, err
	}

	var certDER, keyDER []byte
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch block.Type {
		case "CERTIFICATE":
			certDER = block.Bytes
		case "EC PRIVATE KEY":
			keyDER = block.Bytes
		}
	}
	if certDER == nil || keyDER == nil {
		return tls.Certificate{}, fmt.Errorf("infra: %s is missing a certificate or key block", path)
	}

	key, err := x509.ParseECPrivateKey(keyDER)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("infra: parse server key: %w", err)
	}

	return tls.Certificate{Certificate: [][]byte{certDER}, PrivateKey: key}, nil
}

func createAndSaveServerCertificate(path string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("infra: generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("infra: generate serial: %w", err)
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
		return tls.Certificate{}, fmt.Errorf("infra: create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("infra: marshal server key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return tls.Certificate{}, fmt.Errorf("infra: create server cert dir: %w", err)
	}

	var out []byte
	out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	out = append(out, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})...)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("infra: write server cert: %w", err)
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}
