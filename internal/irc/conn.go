package irc

import (
	"bufio"
	"crypto/tls"
	"net"
	"strconv"
	"time"
)

// dialTimeout bounds the TCP connect; handshakeTimeout bounds the TLS
// handshake. Both are vars so tests can shrink them.
var (
	dialTimeout      = 10 * time.Second
	handshakeTimeout = 15 * time.Second
)

// Dial opens a TCP connection to host:port, wrapping it in TLS when useTLS
// is set. Certificates are verified by default; insecureSkipVerify exists
// only for self-signed bouncers.
//
// Both the TCP connect and the TLS handshake are bounded: without a
// deadline, dialing a host that accepts TCP but never answers (a plaintext
// port with TLS enabled, a filtered address) blocks forever and the UI
// sits on "connecting…" with no explanation.
func Dial(server, host string, port int, useTLS, insecureSkipVerify bool) (*Conn, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	debugf(server, "dialing %s (tls=%v insecure_skip_verify=%v)", addr, useTLS, insecureSkipVerify)

	dialer := &net.Dialer{Timeout: dialTimeout}
	c, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	debugf(server, "tcp connected to %s", addr)
	if !useTLS {
		return wrap(c), nil
	}

	// Bound the handshake separately: against a non-TLS listener the
	// server accepts TCP and then never speaks TLS, which would hang
	// the dial otherwise.
	_ = c.SetDeadline(time.Now().Add(handshakeTimeout))
	tlsConn := tls.Client(c, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // opt-in escape hatch
		MinVersion:         tls.VersionTLS12,
	})
	if err := tlsConn.Handshake(); err != nil {
		c.Close()
		return nil, err
	}
	_ = c.SetDeadline(time.Time{}) // back to blocking I/O for the session
	debugf(server, "tls handshake ok (version %x)", tlsConn.ConnectionState().Version)
	return wrap(tlsConn), nil
}

// Conn is a line-oriented IRC connection over TCP, optionally wrapped in TLS.
type Conn struct {
	raw net.Conn
	r   *bufio.Reader
	w   *bufio.Writer
}

func wrap(c net.Conn) *Conn {
	return &Conn{raw: c, r: bufio.NewReader(c), w: bufio.NewWriter(c)}
}

// Send writes one IRC line (without the trailing CRLF).
func (c *Conn) Send(line string) error {
	if _, err := c.w.WriteString(line + "\r\n"); err != nil {
		return err
	}
	return c.w.Flush()
}

// ReadLine reads one IRC line (without the trailing CRLF).
func (c *Conn) ReadLine() (string, error) {
	line, err := c.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	return line, nil
}

// Close closes the underlying connection.
func (c *Conn) Close() error { return c.raw.Close() }
