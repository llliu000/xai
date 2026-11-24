package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var publicVideoMediaProtection = common.SSRFProtection{AllowedPorts: []int{443}, ApplyIPFilterForDomain: true}

var publicVideoMediaClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext: (&protectedFetchDialer{
			resolver:    net.DefaultResolver,
			dialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
			getProtection: func() (*common.SSRFProtection, bool, error) {
				return &publicVideoMediaProtection, true, nil
			},
		}).DialContext,
		TLSHandshakeTimeout: 3 * time.Second,
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConns:        32,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("Too many media redirects")
		}
		return validatePublicVideoMediaTarget(req.URL)
	},
}

// PublicVideoMediaClient restricts media redirects and connection addresses to
// public HTTPS destinations even when general fetch settings are relaxed.
func PublicVideoMediaClient(rawURL string) (*http.Client, error) {
	target, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if err := validatePublicVideoMediaTarget(target); err != nil {
		return nil, err
	}
	client := *publicVideoMediaClient
	return &client, nil
}

func validatePublicVideoMediaTarget(target *url.URL) error {
	if target.Scheme != "https" || target.User != nil || target.Fragment != "" || target.Opaque != "" ||
		(target.Port() != "" && target.Port() != "443") || target.Hostname() == "" {
		return errors.New("Media requires a public HTTPS URL")
	}
	return publicVideoMediaProtection.ValidateNetworkTarget(target.Hostname(), 443)
}

// ResolvePublicVideoMediaURL checks every redirect with the same DNS-pinned
// transport, then passes the final URL to the upstream task provider.
func ResolvePublicVideoMediaURL(ctx context.Context, raw string) (string, error) {
	resp, err := openPublicVideoMedia(ctx, raw, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return resp.Request.URL.String(), nil
}

func openPublicVideoMedia(ctx context.Context, raw string, partial bool) (*http.Response, error) {
	if len(raw) > 8192 {
		return nil, errors.New("Media URL is too long")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	client, err := PublicVideoMediaClient(raw)
	if err != nil {
		return nil, err
	}
	if partial {
		req.Header.Set("Range", "bytes=0-0")
	} else {
		client.Timeout = 30 * time.Second
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && !(partial && resp.StatusCode == http.StatusPartialContent) {
		_ = resp.Body.Close()
		return nil, errors.New("Media URL did not return a downloadable resource")
	}
	return resp, nil
}
