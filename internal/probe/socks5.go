// Package probe implements network probes used by the control plane.
// This file provides a production-grade SOCKS5 probe with careful timeouts,
// clear error paths, and precise measurement of per-step latencies.
package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/sanverite/simple-packet-logger/internal/core"
)

// Auth holds optional username/password credentials for SOCKS5 "user/pass" auth (method 0x02).
type Auth struct {
	Username string
	Password string
}

// Config controls a single probe execution.
type Config struct {
	// Server is the SOCKS5 endpoint to probe, in "host:port" form.
	// Host may be an IPv4, IPv6 ([...]), or a DNS name. Port must be numeric (1-65535).
	Server string

	// Timeout bounds the entire probe (TCP connect + handshake + connect [+ UDP]).
	// If zero, DefaultTimeout is used.
	Timeout time.Duration

	// Auth, when provided, allows the probe to succeed if the proxy selects "user/pass" auth.
	// If omitted, the probe will only succeed if the proxy accepts "no auth" (method 0x00).
	Auth *Auth

	// ConnectTarget is the destination used in the SOCKS5 CONNECT test.
	// If empty, DefaultConnectTarget is used.
	// Accepts "host:port" where host may be an IP (v4/v6) or a DNS name.
	ConnectTarget string

	// UDPTest requests a minimal UDP ASSOCIATE exchange. A success reply sets UDPOK=true.
	// This does not perform end-to-end UDP payload verification.
	UDPTest bool
}

// Sensible defaults for production probes.
const (
	DefaultTimeout       = 3 * time.Second
	DefaultConnectTarget = "example.com:80"
)

// ProbeSOCKS runs a single SOCKS5 probe against cfg.Server following these steps:
// 1) TCP connect
// 2) SOCKS greeting (negotiate method, optionally do user/pass)
// 3) CONNECT to cfg.ConnectTarget
// 4) (Optional) UDP ASSOCIATE
//
// It returns a core.ProbeSummary with per-step latencies and discovered features.
// Errors indicate probe execution/validation failures; the returned summary includes
// as much signal as possible (e.g., partial latencies, warnings).
func ProbeSOCKS(ctx context.Context, cfg Config) (core.ProbeSummary, error) {
	var (
		warns     []string
		latencies = make(map[string]int64, 4)
		summary   core.ProbeSummary
	)
	defer func() {
		// Populate summary fields that are always set.
		summary.LatenciesMs = latencies
		summary.Warnings = warns
		summary.LastChecked = time.Now()
	}()

	// Validate and normalize inputs.
	serverHost, serverPort, err := splitHostPortStrict(cfg.Server)
	if err != nil {
		return summary, fmt.Errorf("invalid socks server: %w", err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	connectTarget := cfg.ConnectTarget
	if strings.TrimSpace(connectTarget) == "" {
		connectTarget = DefaultConnectTarget
	}
	targetHost, targetPort, err := splitHostPortStrict(connectTarget)
	if err != nil {
		return summary, fmt.Errorf("invalid connect target: %w", err)
	}

	// Use a single deadline for the whole probe; propagate via context and deadlines.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	deadline := time.Now().Add(timeout)

	// Setup dialer and perform TCP connect.
	dialer := &net.Dialer{}
	t0 := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(serverHost, serverPort))
	latencies["tcp_connect"] = millisSince(t0)
	if err != nil {
		warns = append(warns, "tcp connect failed: "+err.Error())
		return summary, err
	}
	defer conn.Close()
	// TCP is reachable once connect succeeded.
	summary.Reachable = true

	// Ensure socket operations respect the global deadline.
	_ = conn.SetDeadline(deadline)

	// Perform SOCKS5 greeting and optional auth.
	handshakeStart := time.Now()
	methodUsed, err := doSocksGreeting(conn, cfg.Auth)
	latencies["socks_handshake"] = millisSince(handshakeStart)
	if err != nil {
		warns = append(warns, "socks handshake failed: "+err.Error())
		return summary, err
	}
	// Greeting (and any required auth) succeeded.
	summary.SocksOK = true

	// Record features based on negotiated method.
	switch methodUsed {
	case 0x00:
		summary.Features.Auth = "none"
	case 0x02:
		summary.Features.Auth = "userpass"
	default:
		// Should not happen because doSocksGreeting enforces methods.
		warns = append(warns, fmt.Sprintf("unexpected method selected: 0x%02x", methodUsed))
	}

	// Build and send CONNECT request.
	connectStart := time.Now()
	atyp, addrBytes, portBytes, ipv6Target, err := encodeSocksAddress(targetHost, targetPort)
	if err != nil {
		warns = append(warns, "invalid connect target encoding: "+err.Error())
		return summary, err
	}
	connectReq := make([]byte, 0, 3+1+len(addrBytes)+2)
	connectReq = append(connectReq, 0x05 /* VER */, 0x01 /* CMD=CONNECT */, 0x00 /* RSV */)
	connectReq = append(connectReq, atyp)
	connectReq = append(connectReq, addrBytes...)
	connectReq = append(connectReq, portBytes...)
	if _, err := conn.Write(connectReq); err != nil {
		warns = append(warns, "write CONNECT failed: "+err.Error())
		return summary, err
	}
	// Read CONNECT reply: VER, REP, RSV, ATYP, BND.ADDR, BND.PORT
	// We read the fixed header first, then discard the bound address as per RFC 1928.
	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		warns = append(warns, "read CONNECT reply header failed: "+err.Error())
		return summary, err
	}
	if hdr[0] != 0x05 {
		warns = append(warns, fmt.Sprintf("unexpected reply version: 0x%02x", hdr[0]))
		return summary, fmt.Errorf("bad connect reply version")
	}
	rep := hdr[1]
	if rep != 0x00 {
		msg := repToString(rep)
		warns = append(warns, "connect failed: "+msg)
		latencies["connect"] = millisSince(connectStart)
		// Not a transport error; return a descriptive error.
		return summary, fmt.Errorf("socks connect failed: %s", msg)
	}
	// Consume the bound address in the reply based on ATYP.
	if err := discardReplyBindAddr(conn, hdr[3]); err != nil {
		warns = append(warns, "read CONNECT reply addr failed: "+err.Error())
		return summary, err
	}
	latencies["connect"] = millisSince(connectStart)

	// CONNECT succeeded.
	summary.ConnectOK = true
	// If we connected to an IPv6 literal successfully, we can claim IPv6 egress support.
	summary.Features.IPv6 = ipv6Target

	// Optionally test UDP ASSOCIATE.
	if cfg.UDPTest {
		udpStart := time.Now()
		udpOK, udpWarn := doUDPAssociate(conn)
		if udpWarn != "" {
			warns = append(warns, udpWarn)
		}
		latencies["udp_associate"] = millisSince(udpStart)
		summary.UDPOK = udpOK
	}
	return summary, nil
}

// doSocksGreeting negotiates a SOCKS5 method and performs optional user/pass auth.
// Returns the method selected by the server and an error if greeting/auth fails.
func doSocksGreeting(conn net.Conn, auth *Auth) (byte, error) {
	// Build methods: always offer "no auth"; offer "user/pass" if credentials provided.
	methods := []byte{0x00}
	if auth != nil {
		methods = append(methods, 0x02)
	}

	// Send greeting: VER, NMETHODS, METHODS...
	buf := make([]byte, 0, 2+len(methods))
	buf = append(buf, 0x05, byte(len(methods)))
	buf = append(buf, methods...)
	if _, err := conn.Write(buf); err != nil {
		return 0, fmt.Errorf("write greeting: %w", err)
	}

	// Read method selection: VER, METHOD
	var sel [2]byte
	if _, err := io.ReadFull(conn, sel[:]); err != nil {
		return 0, fmt.Errorf("read method selection: %w", err)
	}
	if sel[0] != 0x05 {
		return 0, fmt.Errorf("unexpected version in method selection: 0x%02x", sel[0])
	}
	method := sel[1]
	switch method {
	case 0x00: // no auth
		return method, nil
	case 0x02: // username/password
		if auth == nil {
			return method, errors.New("proxy requires username/password but none provided")
		}
		if err := doUserPassAuth(conn, auth); err != nil {
			return method, err
		}
		return method, nil
	case 0xFF:
		return method, errors.New("proxy rejected offered methods")
	default:
		return method, fmt.Errorf("unsupported method selected by proxy: 0x%02x", method)
	}
}

// doUserPassAuth performs RFC 1929 username/password authentication.
func doUserPassAuth(conn net.Conn, auth *Auth) error {
	// Username and password lengths are 0-255 (one byte each). Enforce bounds.
	if len(auth.Username) > 255 || len(auth.Password) > 255 {
		return errors.New("username/password too long (max 255 bytes each)")
	}
	// Build request: VER=0x01, ULEN, UNAME, PLEN, PASSWD
	req := make([]byte, 0, 3+len(auth.Username)+len(auth.Password))
	req = append(req, 0x01, byte(len(auth.Username)))
	req = append(req, auth.Username...)
	req = append(req, byte(len(auth.Password)))
	req = append(req, auth.Password...)
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("write user/pass: %w", err)
	}
	// Read reply: VER=0x01, STATUS=0x00 success
	var rep [2]byte
	if _, err := io.ReadFull(conn, rep[:]); err != nil {
		return fmt.Errorf("read user/pass reply: %w", err)
	}
	if rep[0] != 0x01 {
		return fmt.Errorf("unexpected user/pass reply version: 0x%02x", rep[0])
	}
	if rep[1] != 0x00 {
		return errors.New("user/pass authentication failed")
	}
	return nil
}

// encodeSocksAddress encodes host:port into SOCKS5 ATYP, ADDR, and PORT bytes.
// Returns whether the target host was IPv6 (used to set Features.IPv6).
func encodeSocksAddress(host, port string) (atyp byte, addrBytes []byte, portBytes []byte, ipv6 bool, err error) {
	// Validate port.
	pn, err := strconv.Atoi(port)
	if err != nil || pn < 1 || pn > 65535 {
		return 0, nil, nil, false, fmt.Errorf("invalid port: %q", port)
	}
	portBytes = []byte{byte(pn >> 8), byte(pn & 0xff)}

	// Try to parse as IP first.
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return 0x01, v4, portBytes, false, nil // IPv4
		}
		return 0x04, ip.To16(), portBytes, true, nil // IPv6
	}

	// Domain name
	if len(host) == 0 || len(host) > 255 {
		return 0, nil, nil, false, fmt.Errorf("invalid domain length: %d", len(host))
	}
	addrBytes = make([]byte, 0, 1+len(host))
	addrBytes = append(addrBytes, byte(len(host)))
	addrBytes = append(addrBytes, host...)
	return 0x03, addrBytes, portBytes, false, nil
}

// discardReplyBindAddr consumes BND.ADDR and BND.PORT from a CONNECT/UDP reply based on ATYP.
func discardReplyBindAddr(r io.Reader, atyp byte) error {
	switch atyp {
	case 0x01: // IPv4
		var tmp [4 + 2]byte
		_, err := io.ReadFull(r, tmp[:])
		return err
	case 0x04: // IPv6
		var tmp [16 + 2]byte
		_, err := io.ReadFull(r, tmp[:])
		return err
	case 0x03: // DOMAIN
		// First read length, then that many bytes, then 2 bytes for port.
		var l [1]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return err
		}
		n := int(l[0]) + 2
		if n == 2 {
			// Zero-length domain should not happen; treat as error.
			return errors.New("invalid domain length in reply")
		}
		buf := make([]byte, n)
		_, err := io.ReadFull(r, buf)
		return err
	default:
		return fmt.Errorf("unknown reply ATYP: 0x%02x", atyp)
	}
}

// doUDPAssociate performs a minimal UDP ASSOCIATE exchange to detect support.
// Returns (true, "") on success; (false, warning) on failure, without erroring the whole probe.
func doUDPAssociate(conn net.Conn) (bool, string) {
	// Request: VER=0x05, CMD=0x03 (UDP ASSOCIATE), RSV=0x00, ATYP=IPv4, ADDR=0.0.0.0, PORT=0
	req := []byte{0x05, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if _, err := conn.Write(req); err != nil {
		return false, "write UDP ASSOCIATE failed: " + err.Error()
	}
	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return false, "read UDP ASSOCIATE reply header failed: " + err.Error()
	}
	if hdr[0] != 0x05 {
		return false, fmt.Sprintf("unexpected UDP ASSOCIATE reply version: 0x%02x", hdr[0])
	}
	if hdr[1] != 0x00 {
		return false, "udp associate failed: " + repToString(hdr[1])
	}
	// Discard BND.ADDR/BND.PORT.
	if err := discardReplyBindAddr(conn, hdr[3]); err != nil {
		return false, "read UDP ASSOCIATE bind addr failed: " + err.Error()
	}
	return true, ""
}

// repToString maps REP codes (RFC 1928) to human-readable strings.
func repToString(rep byte) string {
	switch rep {
	case 0x00:
		return "succeeded"
	case 0x01:
		return "general SOCKS server failure"
	case 0x02:
		return "connection not allowed by ruleset"
	case 0x03:
		return "network unreachable"
	case 0x04:
		return "host unreachable"
	case 0x05:
		return "connection refused by destination host"
	case 0x06:
		return "TTL expired"
	case 0x07:
		return "command not supported"
	case 0x08:
		return "address type not supported"
	default:
		return fmt.Sprintf("unknown reply code 0x%02x", rep)
	}
}

// splitHostPortStrict validates "host:port" and returns host and port strings.
// Accepts IPv6 in bracket form, e.g., "[::1]:1080".
func splitHostPortStrict(hp string) (host, port string, err error) {
	host, port, err = net.SplitHostPort(hp)
	if err != nil {
		return "", "", err
	}
	// Validate port is numeric and in range; leave as string on success.
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", "", fmt.Errorf("invalid port %q", port)
	}
	// For bracketed IPv6, net.SplitHostPort already strips brackets.
	// For hostnames, ensure no whitespace.
	if strings.TrimSpace(host) == "" {
		return "", "", errors.New("empty host")
	}
	return host, port, nil
}

// millisSince returns the elapsed milliseconds since t0, clamped at zero.
func millisSince(t0 time.Time) int64 {
	diff := time.Since(t0)
	if diff < 0 {
		return 0
	}
	return diff.Milliseconds()
}
