package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CA file names inside the CA directory.
const (
	CACertFile = "ca.pem"
	CAKeyFile  = "ca-key.pem"
)

// LoadOrCreateCA loads the local root CA from dir, generating a fresh
// ECDSA P-256 CA on first use. goproxy then signs a leaf certificate per host
// on the fly ("dynamic certificate issuance"). The private key never leaves
// dir and is written with 0600 permissions.
func LoadOrCreateCA(dir string) (tls.Certificate, bool, error) {
	certPath := filepath.Join(dir, CACertFile)
	keyPath := filepath.Join(dir, CAKeyFile)
	if ca, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		ca.Leaf, err = x509.ParseCertificate(ca.Certificate[0])
		if err != nil {
			return tls.Certificate{}, false, err
		}
		if time.Now().After(ca.Leaf.NotAfter) {
			return tls.Certificate{}, false, fmt.Errorf("CA in %s expired at %s; delete it to regenerate", dir, ca.Leaf.NotAfter)
		}
		return ca, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return tls.Certificate{}, false, fmt.Errorf("load CA: %w", err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, false, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return tls.Certificate{}, false, err
	}
	host, _ := os.Hostname()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "web-cli local audit CA (" + host + ")", Organization: []string{"web-cli"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return tls.Certificate{}, false, err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return tls.Certificate{}, false, err
	}
	ca, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, false, err
	}
	ca.Leaf, _ = x509.ParseCertificate(ca.Certificate[0])
	return ca, true, nil
}

// SPKIHash returns base64(sha256(SubjectPublicKeyInfo)) of the CA, the value
// Chrome accepts in --ignore-certificate-errors-spki-list.
func SPKIHash(ca tls.Certificate) string {
	sum := sha256.Sum256(ca.Leaf.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// certCache implements goproxy.CertStorage so each host is signed only once.
type certCache struct {
	mu sync.Mutex
	m  map[string]*tls.Certificate
}

func (c *certCache) Fetch(host string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cert, ok := c.m[host]; ok {
		return cert, nil
	}
	cert, err := gen()
	if err != nil {
		return nil, err
	}
	c.m[host] = cert
	return cert, nil
}
