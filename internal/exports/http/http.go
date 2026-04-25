// Package http provides generic HTTP client functionality without reflection.
package http

import (
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client represents an HTTP client with a base URL and reusable configuration.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	AuthHeader string
}

// Options holds configuration for an HTTP request.
type Options struct {
	Timeout time.Duration
	TLS     *tls.Config
	CookieJar http.CookieJar
	Headers map[string]string
	Body    io.Reader
	Auth    *Auth
}

// Auth holds authentication credentials.
type Auth struct {
	Type     string // "basic" or "bearer"
	Username string
	Password string
	Token    string
}

// NewClient creates a new HTTP client with the given base URL and options.
// If opts is nil, default values are used (5 second timeout, no TLS config).
func NewClient(baseURL string, opts *Options) *Client {
	timeout := 5 * time.Second
	var tlsCfg *tls.Config
	if opts != nil {
		if opts.Timeout > 0 {
			timeout = opts.Timeout
		}
		tlsCfg = opts.TLS
	}
	tr := &http.Transport{
		TLSClientConfig: tlsCfg,
	}
	jar := http.CookieJar(nil)
	if opts != nil && opts.CookieJar != nil {
		jar = opts.CookieJar
	}
	hc := &http.Client{
		Transport: tr,
		Timeout:   timeout,
		Jar:       jar,
	}
	c := &Client{
		BaseURL:    baseURL,
		HTTPClient: hc,
	}
	if opts != nil && opts.Auth != nil {
		switch opts.Auth.Type {
		case "basic":
			c.AuthHeader = "Basic " + basicAuth(opts.Auth.Username, opts.Auth.Password)
		case "bearer":
			c.AuthHeader = "Bearer " + opts.Auth.Token
		}
	}
	return c
}

// Request performs an HTTP request with the given method, path, and request options.
// The path is joined with the client's BaseURL (unless path is already an absolute URL).
// If ropts is nil, no additional headers or body are added.
// Returns the HTTP response, or an error.
func (c *Client) Request(method, path string, ropts *Options) (*http.Response, error) {
	full, err := joinURL(c.BaseURL, path)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	var headers map[string]string
	var auth *Auth
	if ropts != nil {
		body = ropts.Body
		headers = ropts.Headers
		auth = ropts.Auth
	}
	req, err := http.NewRequest(method, full, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %v", err)
	}
	// Apply headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Apply authentication (overrides client-wide auth if provided)
	if auth != nil {
		switch auth.Type {
		case "basic":
			req.SetBasicAuth(auth.Username, auth.Password)
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+auth.Token)
		}
	} else if c.AuthHeader != "" {
		req.Header.Set("Authorization", c.AuthHeader)
	}
	return c.HTTPClient.Do(req)
}

// Get performs a GET request.
func (c *Client) Get(path string, opts *Options) (*http.Response, error) {
	return c.Request("GET", path, opts)
}

// Post performs a POST request with the given body.
func (c *Client) Post(path string, body io.Reader, opts *Options) (*http.Response, error) {
	if opts == nil {
		opts = &Options{}
	}
	opts.Body = body
	return c.Request("POST", path, opts)
}

// ResponseResult holds a simplified representation of an HTTP response.
type ResponseResult struct {
	Status  int
	Body    string
	Headers map[string]string
}

// Do performs a request and reads the entire response body, returning a ResponseResult.
// The response body is closed after reading.
func (c *Client) Do(method, path string, opts *Options) (*ResponseResult, error) {
	resp, err := c.Request(method, path, opts)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}
	headers := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		headers[k] = strings.Join(v, ",")
	}
	return &ResponseResult{
		Status:  resp.StatusCode,
		Body:    string(bodyBytes),
		Headers: headers,
	}, nil
}

// SimpleRequest is a convenience function that creates a one-off client and performs a request.
// It uses default timeout (5 seconds) and no TLS config.
func SimpleRequest(method, urlStr string, opts *Options) (*ResponseResult, error) {
	client := NewClient("", opts) // baseURL empty, path must be absolute
	return client.Do(method, urlStr, opts)
}

// joinURL combines a base URL and a path, handling absolute paths correctly.
func joinURL(base, path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path, nil
	}
	if base == "" {
		return "", errors.New("base URL required for relative path")
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return u.String(), nil
}

// BasicAuth returns the base64-encoded "username:password" string suitable for HTTP Basic Authentication.
func BasicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func basicAuth(username, password string) string {
	return BasicAuth(username, password)
}