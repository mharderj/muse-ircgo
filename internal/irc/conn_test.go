package irc

import (
	"net"
	"strconv"
	"testing"
	"time"
)

// A TLS dial against a listener that never speaks TLS must fail fast,
// not hang until the OS gives up. (This is the "stuck at connecting…"
// scenario: e.g. tls=true pointed at a plaintext ZNC port.)
func TestDialTLSAgainstPlaintextTimesOut(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
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
			// Accept and say nothing: like a plaintext port
			// receiving a TLS ClientHello.
			time.Sleep(2 * time.Second)
			c.Close()
		}
	}()

	oldD, oldH := dialTimeout, handshakeTimeout
	dialTimeout, handshakeTimeout = 200*time.Millisecond, 300*time.Millisecond
	defer func() { dialTimeout, handshakeTimeout = oldD, oldH }()

	_, port := splitHostPort(t, ln.Addr().String())
	start := time.Now()
	_, err = Dial("test", "127.0.0.1", port, true, true)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected handshake error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("dial hung: took %v", elapsed)
	}
	t.Logf("failed fast as expected: %v (%v)", err, elapsed)
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
