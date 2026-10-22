package docker

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPlanFor(t *testing.T) {
	cases := []struct {
		in           []string
		native, then []string
	}{
		{[]string{"run", "nginx"}, nil, nil},
		{[]string{"build", "-t", "app", "."}, []string{"build", "-t", "app", "."}, nil},
		{[]string{"buildx", "build", "."}, []string{"buildx", "build", "."}, nil},
		{[]string{"image", "build", "."}, []string{"image", "build", "."}, nil},
		{[]string{"compose", "-f", "dev.yaml", "build"}, []string{"compose", "-f", "dev.yaml", "build"}, nil},
		{[]string{"compose", "up", "-d"}, nil, nil},
		{
			[]string{"compose", "-p", "x", "up", "-d", "--build", "api"},
			[]string{"compose", "-p", "x", "build"},
			[]string{"compose", "-p", "x", "up", "-d", "api"},
		},
		{[]string{"compose", "run", "api", "--", "echo", "--build"}, nil, nil},
	}
	for _, c := range cases {
		p := planFor(c.in)
		if c.native == nil {
			if p != nil {
				t.Errorf("%q : ne doit pas passer par le client Windows", c.in)
			}
			continue
		}
		if p == nil || !reflect.DeepEqual(p.native, c.native) || !reflect.DeepEqual(p.then, c.then) {
			t.Errorf("%q : obtenu %+v", c.in, p)
		}
	}
}

func TestSplitCompose(t *testing.T) {
	g, sub, rest := splitCompose([]string{"--file", "a.yaml", "--env-file=.env", "up", "-d"})
	if !reflect.DeepEqual(g, []string{"--file", "a.yaml", "--env-file=.env"}) || sub != "up" || !reflect.DeepEqual(rest, []string{"-d"}) {
		t.Fatalf("obtenu %q %q %q", g, sub, rest)
	}
}

func TestChecksumFor(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	file := []byte(strings.Repeat("cd", 32) + "  other.exe\n" + strings.ToUpper(sum) + " *buildx-v1.windows-amd64.exe\n")
	if got, ok := checksumFor(file, "buildx-v1.windows-amd64.exe"); !ok || got != sum {
		t.Fatalf("obtenu %q %v", got, ok)
	}
	if _, ok := checksumFor(file, "absent.exe"); ok {
		t.Fatal("fichier absent")
	}
}

func TestPickLatestStatic(t *testing.T) {
	page := `<a href="docker-29.9.2.zip">docker-29.9.2.zip</a> <a href="docker-29.10.0.zip">x</a> docker-17.09.0-ce.zip docker-29.9.10.zip`
	if v, err := pickLatestStatic(page); err != nil || v != "29.10.0" {
		t.Fatalf("obtenu %q %v", v, err)
	}
	if _, err := pickLatestStatic("rien"); err == nil {
		t.Fatal("page vide : erreur attendue")
	}
}

func TestAuthsOnly(t *testing.T) {
	out, ok := authsOnly([]byte(`{"auths":{"ghcr.io":{"auth":"eDp5"}},"credsStore":"desktop","currentContext":"x"}`))
	if !ok || !strings.Contains(string(out), "ghcr.io") || strings.Contains(string(out), "credsStore") || strings.Contains(string(out), "currentContext") {
		t.Fatalf("obtenu %s", out)
	}
}

func TestExtractFromZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("docker/docker.exe")
	w.Write([]byte("MZ"))
	zw.Close()
	if got, err := extractFromZip(buf.Bytes(), "docker/docker.exe"); err != nil || string(got) != "MZ" {
		t.Fatalf("obtenu %q %v", got, err)
	}
}

// Le client et le moteur se reconnaissent, et un certificat d'une autre autorité est refusé.
func TestCertsHandshake(t *testing.T) {
	dir := t.TempDir()
	cli, srv := filepath.Join(dir, "certs"), filepath.Join(dir, "certs", "server")
	if err := os.MkdirAll(cli, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureCertsIn(cli, srv); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(cli, "cert.pem"))
	if err := ensureCertsIn(cli, srv); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(filepath.Join(cli, "cert.pem")); !bytes.Equal(before, after) {
		t.Fatal("les certificats existants ne doivent pas être recréés")
	}

	pool := func(path string) *x509.CertPool {
		p := x509.NewCertPool()
		b, _ := os.ReadFile(path)
		if !p.AppendCertsFromPEM(b) {
			t.Fatal("CA illisible")
		}
		return p
	}
	srvPair, err := tls.LoadX509KeyPair(filepath.Join(srv, "server-cert.pem"), filepath.Join(srv, "server-key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	cliPair, err := tls.LoadX509KeyPair(filepath.Join(cli, "cert.pem"), filepath.Join(cli, "key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{srvPair}, ClientCAs: pool(filepath.Join(srv, "ca.pem")),
		ClientAuth: tls.RequireAndVerifyClientCert,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.(*tls.Conn).Handshake()
			c.Close()
		}
	}()
	dial := func(cert tls.Certificate) error {
		c, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
			RootCAs: pool(filepath.Join(cli, "ca.pem")), Certificates: []tls.Certificate{cert}, ServerName: "127.0.0.1",
		})
		if err == nil {
			_, err = c.Read(make([]byte, 1)) // TLS 1.3 : le refus du client arrive à la première lecture
			if err != nil && strings.Contains(err.Error(), "EOF") {
				err = nil
			}
			c.Close()
		}
		return err
	}
	if err := dial(cliPair); err != nil {
		t.Fatalf("le client doit être accepté : %v", err)
	}

	other := t.TempDir()
	if err := ensureCertsIn(other, filepath.Join(other, "server")); err != nil {
		t.Fatal(err)
	}
	stranger, err := tls.LoadX509KeyPair(filepath.Join(other, "cert.pem"), filepath.Join(other, "key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if dial(stranger) == nil {
		t.Fatal("un certificat d'une autre autorité doit être refusé")
	}
}
