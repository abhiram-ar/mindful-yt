// Package proxy runs a private HTTP proxy that exists only while mindful-yt runs;
// yt-dlp is pointed at it with --proxy. It looks blocked YouTube hostnames up
// over DNS-over-HTTPS and relays bytes. TLS stays end to end between yt-dlp
// and YouTube, so the proxy never sees the traffic itself. A random password
// per run keeps other programs on the machine from using it.
package proxy

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"time"
)

type Proxy struct {
	url    string // what yt-dlp gets as --proxy, password included
	auth   string // the Proxy-Authorization header that must match
	server *http.Server
	dialer net.Dialer
	lookup LookupFunc
}

// Start listens on a random local port until Close.
func Start(lookup LookupFunc) (*Proxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		listener.Close()
		return nil, err
	}
	password := hex.EncodeToString(secret)
	p := &Proxy{
		url:    "http://mindful-yt:" + password + "@" + listener.Addr().String(),
		auth:   "Basic " + base64.StdEncoding.EncodeToString([]byte("mindful-yt:"+password)),
		dialer: net.Dialer{Timeout: 15 * time.Second},
		lookup: lookup,
	}
	p.server = &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go p.server.Serve(listener)
	return p, nil
}

// URL is the proxy's address with this run's password, for yt-dlp's --proxy.
func (p *Proxy) URL() string { return p.url }

func (p *Proxy) Close() error { return p.server.Close() }

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Proxy-Authorization")), []byte(p.auth)) != 1 {
		w.Header().Set("Proxy-Authenticate", `Basic realm="mindful-yt"`)
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}
	if r.Method != http.MethodConnect {
		http.Error(w, "only HTTPS tunnels are supported", http.StatusMethodNotAllowed)
		return
	}

	upstream, err := p.dial(r.Context(), r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	client, buffered, err := http.NewResponseController(w).Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	// The buffered reader may already hold bytes sent right after the CONNECT.
	go func() {
		io.Copy(upstream, buffered.Reader)
		upstream.Close()
		client.Close()
	}()
	io.Copy(client, upstream)
	upstream.Close()
	client.Close()
}

func (p *Proxy) dial(ctx context.Context, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := p.lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return p.dialer.DialContext(ctx, "tcp", addr)
	}
	var lastErr error
	for _, ip := range ips {
		conn, err := p.dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
