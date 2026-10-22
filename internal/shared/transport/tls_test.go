package transport

import (
	"context"
	"crypto/tls"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPinnedTLS(t *testing.T) {
	dir := t.TempDir()
	crt, key := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := Generate(crt, key, "node"); err != nil {
		t.Fatal(err)
	}
	pin, err := Fingerprint(crt)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.LoadX509KeyPair(crt, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		pin string
		ok  bool
	}{{pin, true}, {strings.Repeat("0", 64), false}, {"", false}} {
		cfg, err := ClientTLS(tc.pin)
		if err != nil {
			t.Fatal(err)
		}
		cfg.ServerName = "localhost"
		a, b := net.Pipe()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		done := make(chan error, 1)
		go func() {
			done <- tls.Server(a, &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS13}).HandshakeContext(ctx)
		}()
		err = tls.Client(b, cfg).HandshakeContext(ctx)
		a.Close()
		b.Close()
		cancel()
		<-done
		if (err == nil) != tc.ok {
			t.Fatalf("pin %q: err=%v", tc.pin, err)
		}
	}
	if _, err := ClientTLS("bad-pin"); err == nil {
		t.Fatal("malformed pin accepted")
	}
}
