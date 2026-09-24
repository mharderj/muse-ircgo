package irc

import (
	"bufio"
	"crypto/tls"
	"net"
	"strconv"
)

// Conn is a line-oriented IRC connection over TCP, optionally wrapped in TLS.
type Conn struct {
	raw net.Conn
	r   *bufio.Reader
	w   *bufio.Writer
}

// Dial opens a TCP connection to host:port, wrapping it in TLS when useTLS
// is set. Certificates are verified by default; insecureSkipVerify exists
// only for self-signed bouncers.
func Dial(host string, port int, useTLS, insecureSkipVerify bool) (*Conn, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	if !useTLS {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			return nil, err
		}
		return wrap(c), nil
	}
	c, err := tls.Dial("tcp", addr, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // opt-in escape hatch
		MinVersion:         tls.VersionTLS12,
	})
	if err != nil {
		return nil, err
	}
	return wrap(c), nil
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
