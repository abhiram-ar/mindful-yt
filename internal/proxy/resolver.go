package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// LookupFunc returns the addresses to dial for host, or none to use the
// system resolver.
type LookupFunc func(ctx context.Context, host string) ([]string, error)

var (
	overrideDomains = []string{"youtube.com", "youtu.be"}
	dohEndpoints    = []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/resolve"}
)

func isOverridden(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, d := range overrideDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// YouTubeLookup sends YouTube hostnames to r and everything else to the
// normal system lookup.
func YouTubeLookup(r *Resolver) LookupFunc {
	return func(ctx context.Context, host string) ([]string, error) {
		if !isOverridden(host) {
			return nil, nil
		}
		return r.Lookup(ctx, strings.ToLower(strings.TrimSuffix(host, ".")))
	}
}

// Resolver asks DNS-over-HTTPS servers for IPv4 addresses and remembers the
// answers for the rest of the run.
type Resolver struct {
	client    *http.Client
	endpoints []string

	mu    sync.Mutex
	cache map[string][]string
}

func NewResolver() *Resolver {
	return &Resolver{
		client:    &http.Client{Timeout: 10 * time.Second},
		endpoints: dohEndpoints,
		cache:     map[string][]string{},
	}
}

// Lookup returns the IPv4 addresses of host, trying each server in turn. The
// servers are IP literals, so asking them never needs a DNS lookup of its own.
func (r *Resolver) Lookup(ctx context.Context, host string) ([]string, error) {
	r.mu.Lock()
	ips, ok := r.cache[host]
	r.mu.Unlock()
	if ok {
		return ips, nil
	}
	var problems []string
	for _, endpoint := range r.endpoints {
		ips, err := r.query(ctx, endpoint, host)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", endpoint, err))
			continue
		}
		r.mu.Lock()
		r.cache[host] = ips
		r.mu.Unlock()
		return ips, nil
	}
	return nil, fmt.Errorf("DNS-over-HTTPS lookup of %s failed (%s)", host, strings.Join(problems, "; "))
}

func (r *Resolver) query(ctx context.Context, endpoint, host string) ([]string, error) {
	query := url.Values{"name": {host}, "type": {"A"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var body struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	var ips []string
	for _, a := range body.Answer {
		if a.Type == 1 && net.ParseIP(a.Data) != nil {
			ips = append(ips, a.Data)
		}
	}
	if len(ips) == 0 {
		return nil, errors.New("no addresses")
	}
	return ips, nil
}
