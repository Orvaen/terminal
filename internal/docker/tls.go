package docker

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Le moteur écoute aussi sur 127.0.0.1:EnginePort dans la machine (relayé par WSL vers le localhost de
// Windows) pour que le client docker Windows lui parle sans passer par le partage de fichiers /mnt/c.
// La connexion est protégée par TLS avec certificat client : seul ton compte Windows, qui peut lire
// les clés dans %LOCALAPPDATA%, peut utiliser le moteur. Les autres comptes du PC n'y ont pas accès.
const EnginePort = "23760"

// Fichiers côté client (dossier certsDir) et côté moteur (dossier serverCertsDir, copiés dans la machine).
var (
	clientCertFiles = []string{"ca.pem", "cert.pem", "key.pem"}
	serverCertFiles = []string{"ca.pem", "server-cert.pem", "server-key.pem"}
)

func certsDir() string       { return filepath.Join(clientDir(), "certs") }
func serverCertsDir() string { return filepath.Join(certsDir(), "server") }

// ensureCerts crée les certificats s'il en manque un (tous sont alors recréés ensemble).
func ensureCerts() error { return ensureCertsIn(certsDir(), serverCertsDir()) }

func ensureCertsIn(clientDir, serverDir string) error {
	complete := true
	for _, f := range clientCertFiles {
		complete = complete && fileExists(filepath.Join(clientDir, f))
	}
	for _, f := range serverCertFiles {
		complete = complete && fileExists(filepath.Join(serverDir, f))
	}
	if complete {
		return nil
	}
	files, err := generateCerts(time.Now())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(serverDir, 0o700); err != nil {
		return err
	}
	for name, data := range files {
		dir := clientDir
		if name == "server-cert.pem" || name == "server-key.pem" || name == "server-ca.pem" {
			dir = serverDir
			if name == "server-ca.pem" {
				name = "ca.pem"
			}
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// generateCerts crée une autorité locale, un certificat serveur (127.0.0.1, localhost) et un certificat client.
// La clé de l'autorité n'est pas gardée : on recrée tout si besoin.
func generateCerts(now time.Time) (map[string][]byte, error) {
	newKey := func() (*ecdsa.PrivateKey, error) { return ecdsa.GenerateKey(elliptic.P256(), rand.Reader) }
	serial := func() (*big.Int, error) { return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127)) }
	notBefore, notAfter := now.Add(-time.Hour), now.AddDate(10, 0, 0)

	caKey, err := newKey()
	if err != nil {
		return nil, err
	}
	sn, err := serial()
	if err != nil {
		return nil, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber: sn, Subject: pkix.Name{CommonName: "Orvaen Terminal Docker CA"},
		NotBefore: notBefore, NotAfter: notAfter,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, BasicConstraintsValid: true, IsCA: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}

	leaf := func(cn string, usage x509.ExtKeyUsage, server bool) (certPEM, keyPEM []byte, err error) {
		key, err := newKey()
		if err != nil {
			return nil, nil, err
		}
		sn, err := serial()
		if err != nil {
			return nil, nil, err
		}
		tmpl := &x509.Certificate{
			SerialNumber: sn, Subject: pkix.Name{CommonName: cn},
			NotBefore: notBefore, NotAfter: notAfter,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
		}
		if server {
			tmpl.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
			tmpl.DNSNames = []string{"localhost"}
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
		if err != nil {
			return nil, nil, err
		}
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, nil, err
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
	}
	srvCert, srvKey, err := leaf("orvaen-docker", x509.ExtKeyUsageServerAuth, true)
	if err != nil {
		return nil, err
	}
	cliCert, cliKey, err := leaf("orvaen-terminal", x509.ExtKeyUsageClientAuth, false)
	if err != nil {
		return nil, err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return map[string][]byte{
		"ca.pem": caPEM, "cert.pem": cliCert, "key.pem": cliKey,
		"server-ca.pem": caPEM, "server-cert.pem": srvCert, "server-key.pem": srvKey,
	}, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
