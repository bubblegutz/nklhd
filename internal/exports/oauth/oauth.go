// Package oauth provides OAuth 2.0 client functionality (device flow and authorization code).
package oauth

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	exportsjson "github.com/bubblegutz/nklhd/internal/exports/json"
)

// Client represents an OAuth 2.0 client.
type Client struct {
	ClientID  string
	TokenURL  string
	DeviceURL string
	Client    *http.Client
	// TokenPersistFunc can be set to persist tokens after successful exchange.
	TokenPersistFunc func(map[string]interface{})
}

// Config holds OAuth client configuration.
type Config struct {
	ClientID  string
	TokenURL  string
	DeviceURL string
	Timeout   time.Duration
	TLS       *tls.Config
}

// NewClient creates a new OAuth client with the given configuration.
func NewClient(cfg *Config) *Client {
	timeout := 5 * time.Second
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	tr := &http.Transport{
		TLSClientConfig: cfg.TLS,
	}
	hc := &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}
	return &Client{
		ClientID:  cfg.ClientID,
		TokenURL:  cfg.TokenURL,
		DeviceURL: cfg.DeviceURL,
		Client:    hc,
	}
}

// DeviceFlowStart initiates the OAuth 2.0 Device Authorization Flow.
// Returns a map with device_code, user_code, verification_uri, etc.
func (c *Client) DeviceFlowStart(scope string) (map[string]interface{}, error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	if scope != "" {
		form.Set("scope", scope)
	}
	resp, err := c.Client.PostForm(c.DeviceURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	val, err := exportsjson.Decode(string(body))
	if err != nil {
		return nil, err
	}
	data, ok := val.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected JSON object")
	}
	return data, nil
}

// DevicePoll polls the token endpoint for device flow until authorization is complete.
// interval is seconds between attempts, maxAttempts is total attempts.
// Returns token response or error.
func (c *Client) DevicePoll(deviceCode string, interval, maxAttempts int) (map[string]interface{}, error) {
	for i := 0; i < maxAttempts; i++ {
		form := url.Values{}
		form.Set("client_id", c.ClientID)
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("device_code", deviceCode)
		resp, err := c.Client.PostForm(c.TokenURL, form)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		val, err := exportsjson.Decode(string(body))
		if err != nil {
			return nil, err
		}
		data, ok := val.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("expected JSON object")
		}
		if _, ok := data["access_token"]; ok {
			if c.TokenPersistFunc != nil {
				c.TokenPersistFunc(data)
			}
			return data, nil
		}
		// Check for OAuth errors
		if errStr, ok := data["error"].(string); ok {
			if errStr == "authorization_pending" {
				time.Sleep(time.Duration(interval) * time.Second)
				continue
			}
			// Other error
			if errDesc, ok := data["error_description"].(string); ok {
				return nil, &OAuthError{Err: errStr, Description: errDesc}
			}
			return nil, &OAuthError{Err: errStr}
		}
		return nil, &OAuthError{Err: "unknown_error"}
	}
	return nil, &OAuthError{Err: "timeout", Description: "device flow polling timed out"}
}

// AuthCodeURL constructs the authorization code URL for the user to visit.
func (c *Client) AuthCodeURL(authEndpoint, scope, state string) (string, error) {
	u, err := url.Parse(authEndpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", c.ClientID)
	if scope != "" {
		q.Set("scope", scope)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeCode exchanges an authorization code for an access token.
func (c *Client) ExchangeCode(code, redirectURI, clientSecret string) (map[string]interface{}, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", c.ClientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	resp, err := c.Client.PostForm(c.TokenURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	val, err := exportsjson.Decode(string(body))
	if err != nil {
		return nil, err
	}
	data, ok := val.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected JSON object")
	}
	if c.TokenPersistFunc != nil {
		c.TokenPersistFunc(data)
	}
	return data, nil
}

// OAuthError represents an OAuth protocol error.
type OAuthError struct {
	Err         string
	Description string
}

func (e *OAuthError) Error() string {
	if e.Description != "" {
		return e.Err + ": " + e.Description
	}
	return e.Err
}

// AttachToHTTPClient attaches the OAuth client's token to an HTTP client's transport.
// This is a simple implementation that adds an Authorization header.
// For production, consider using oauth2.Transport.
func (c *Client) AttachToHTTPClient(hc *http.Client, token string) error {
	// This is a placeholder; real implementation would wrap the transport.
	// For now, we assume the caller will set the header themselves.
	return nil
}