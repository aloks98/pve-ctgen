// Package transport provides TLS identity provisioning and certificate pinning.
package transport

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"github.com/aloks98/pve-ctgen/internal/shared/fileutil"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

func Generate(certPath, keyPath, node string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: node}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(2, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}}
	if node != "" {
		cert.DNSNames = append(cert.DNSNames, node)
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, pub, priv)
	if err != nil {
		return err
	}
	key, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	if err = fileutil.AtomicWrite(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		return err
	}
	return fileutil.AtomicWrite(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
}
func Fingerprint(certPath string) (string, error) {
	b, err := os.ReadFile(certPath)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return "", fmt.Errorf("invalid PEM certificate")
	}
	if _, err = x509.ParseCertificate(block.Bytes); err != nil {
		return "", err
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:]), nil
}
func ClientTLS(fingerprint string) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	if fingerprint == "" {
		return cfg, nil
	} // Use the system's CA roots for managed certificates.
	want, err := hex.DecodeString(strings.ToLower(fingerprint))
	if err != nil || len(want) != sha256.Size {
		return nil, fmt.Errorf("TLS fingerprint must be a SHA-256 hex digest")
	}
	// The token authenticates the exact server certificate instead of a CA.
	// Never permit an unverified certificate when conventional verification is disabled.
	cfg.InsecureSkipVerify = true
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return fmt.Errorf("missing pinned server certificate")
		}
		cert := cs.PeerCertificates[0]
		got := sha256.Sum256(cert.Raw)
		if !strings.EqualFold(hex.EncodeToString(got[:]), fingerprint) {
			return fmt.Errorf("server certificate fingerprint mismatch")
		}
		if time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
			return fmt.Errorf("server certificate is not valid at the current time")
		}
		return nil
	}
	return cfg, nil
}
