package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOnlyYouTubeHostsAreOverridden(t *testing.T) {
	for _, host := range []string{"youtube.com", "www.youtube.com", "M.YouTube.com.", "youtu.be"} {
		if !isOverridden(host) {
			t.Errorf("%s should be overridden", host)
		}
	}
	for _, host := range []string{"notyoutube.com", "rr1---sn-abc.googlevideo.com", "1.1.1.1"} {
		if isOverridden(host) {
			t.Errorf("%s should not be overridden", host)
		}
	}
}

func TestResolverFallsBackAndCaches(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("name") != "www.youtube.com" || q.Get("type") != "A" ||
			r.Header.Get("Accept") != "application/dns-json" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"Answer":[{"type":5,"data":"youtube-ui.l.google.com."},{"type":1,"data":"203.0.113.7"}]}`)
	}))
	defer server.Close()

	r := &Resolver{
		client:    server.Client(),
		endpoints: []string{"http://127.0.0.1:1/dns-query", server.URL}, // first one is down
		cache:     map[string][]string{},
	}
	for range 2 {
		ips, err := r.Lookup(context.Background(), "www.youtube.com")
		if err != nil || len(ips) != 1 || ips[0] != "203.0.113.7" {
			t.Fatalf("got %v, %v", ips, err)
		}
	}
	if calls != 1 {
		t.Errorf("asked %d times; the second lookup should come from the cache", calls)
	}
}

func TestProxyTunnelsToOverriddenAddressWithPasswordOnly(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello from the fake youtube")
	}))
	defer server.Close()
	_, port, _ := net.SplitHostPort(server.Listener.Addr().String())

	p, err := Start(func(ctx context.Context, host string) ([]string, error) {
		if host == "www.youtube.com" {
			return []string{"127.0.0.1"}, nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	get := func(proxyURL string) (string, error) {
		u, _ := url.Parse(proxyURL)
		client := &http.Client{Transport: &http.Transport{
			Proxy: http.ProxyURL(u),
			// The test certificate is for example.com; the real one would be YouTube's.
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "example.com"},
		}}
		resp, err := client.Get("https://www.youtube.com:" + port + "/")
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return string(body), err
	}

	body, err := get(p.URL())
	if err != nil || body != "hello from the fake youtube" {
		t.Fatalf("with password: got %q, %v", body, err)
	}

	noPassword, _ := url.Parse(p.URL())
	noPassword.User = nil
	if _, err := get(noPassword.String()); err == nil || !strings.Contains(err.Error(), "Proxy Authentication Required") {
		t.Fatalf("without password: got %v", err)
	}
}
