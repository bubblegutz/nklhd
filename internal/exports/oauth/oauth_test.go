package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	cfg := &Config{
		ClientID:  "test-client",
		TokenURL:  "https://example.com/token",
		DeviceURL: "https://example.com/device",
		Timeout:   10 * time.Second,
	}
	client := NewClient(cfg)
	if client.ClientID != cfg.ClientID {
		t.Errorf("ClientID mismatch: got %s, want %s", client.ClientID, cfg.ClientID)
	}
	if client.TokenURL != cfg.TokenURL {
		t.Errorf("TokenURL mismatch: got %s, want %s", client.TokenURL, cfg.TokenURL)
	}
	if client.DeviceURL != cfg.DeviceURL {
		t.Errorf("DeviceURL mismatch: got %s, want %s", client.DeviceURL, cfg.DeviceURL)
	}
	if client.Client == nil {
		t.Error("HTTP client is nil")
	}
}

func TestDeviceFlowStartSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/device" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"device_code": "device123",
			"user_code": "USER-CODE",
			"verification_uri": "https://example.com/verify",
			"interval": 5
		}`))
	}))
	defer server.Close()

	cfg := &Config{
		ClientID:  "client-id",
		TokenURL:  "https://example.com/token",
		DeviceURL: server.URL + "/device",
	}
	client := NewClient(cfg)
	data, err := client.DeviceFlowStart("scope1 scope2")
	if err != nil {
		t.Fatalf("DeviceFlowStart failed: %v", err)
	}
	if data["device_code"] != "device123" {
		t.Errorf("device_code mismatch: got %v", data["device_code"])
	}
	if data["user_code"] != "USER-CODE" {
		t.Errorf("user_code mismatch: got %v", data["user_code"])
	}
	if data["verification_uri"] != "https://example.com/verify" {
		t.Errorf("verification_uri mismatch: got %v", data["verification_uri"])
	}
	// interval may be int or float64
	if !(data["interval"] == 5 || data["interval"] == 5.0) {
		t.Errorf("interval mismatch: got %v", data["interval"])
	}
}

func TestDeviceFlowStartError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer server.Close()

	cfg := &Config{
		ClientID:  "client-id",
		DeviceURL: server.URL,
	}
	client := NewClient(cfg)
	data, err := client.DeviceFlowStart("")
	if err != nil {
		t.Fatalf("DeviceFlowStart returned unexpected error: %v", err)
	}
	if data["error"] != "invalid_client" {
		t.Errorf("expected error field 'invalid_client', got %v", data["error"])
	}
}

func TestDevicePollSuccess(t *testing.T) {
	var attempt int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			// First attempt: authorization_pending
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		// Second attempt: success
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"access_token": "token123",
			"token_type": "bearer",
			"expires_in": 3600
		}`))
	}))
	defer server.Close()

	cfg := &Config{
		ClientID: "client-id",
		TokenURL: server.URL,
	}
	client := NewClient(cfg)
	data, err := client.DevicePoll("device123", 1, 3)
	if err != nil {
		t.Fatalf("DevicePoll failed: %v", err)
	}
	if data["access_token"] != "token123" {
		t.Errorf("access_token mismatch: got %v", data["access_token"])
	}
	if data["token_type"] != "bearer" {
		t.Errorf("token_type mismatch: got %v", data["token_type"])
	}
	// expires_in may be int or float64
	if !(data["expires_in"] == 3600 || data["expires_in"] == 3600.0) {
		t.Errorf("expires_in mismatch: got %v", data["expires_in"])
	}
}

func TestDevicePollTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer server.Close()

	cfg := &Config{
		ClientID: "client-id",
		TokenURL: server.URL,
	}
	client := NewClient(cfg)
	_, err := client.DevicePoll("device123", 1, 2)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	oauthErr, ok := err.(*OAuthError)
	if !ok {
		t.Fatalf("expected OAuthError, got %T", err)
	}
	if oauthErr.Err != "timeout" {
		t.Errorf("expected error 'timeout', got %v", oauthErr.Err)
	}
}

func TestDevicePollOtherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"access_denied","error_description":"User denied"}`))
	}))
	defer server.Close()

	cfg := &Config{
		ClientID: "client-id",
		TokenURL: server.URL,
	}
	client := NewClient(cfg)
	_, err := client.DevicePoll("device123", 1, 1)
	if err == nil {
		t.Fatal("expected error")
	}
	oauthErr, ok := err.(*OAuthError)
	if !ok {
		t.Fatalf("expected OAuthError, got %T", err)
	}
	if oauthErr.Err != "access_denied" {
		t.Errorf("expected error 'access_denied', got %v", oauthErr.Err)
	}
	if oauthErr.Description != "User denied" {
		t.Errorf("expected description 'User denied', got %v", oauthErr.Description)
	}
}

func TestAuthCodeURL(t *testing.T) {
	cfg := &Config{
		ClientID: "client-id",
	}
	client := NewClient(cfg)
	url, err := client.AuthCodeURL("https://example.com/auth", "read write", "state123")
	if err != nil {
		t.Fatalf("AuthCodeURL failed: %v", err)
	}
	expected := "https://example.com/auth?client_id=client-id&scope=read+write&state=state123"
	if url != expected {
		t.Errorf("URL mismatch:\n got %s\n want %s", url, expected)
	}
}

func TestExchangeCodeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"access_token": "token456",
			"refresh_token": "refresh789",
			"scope": "read"
		}`))
	}))
	defer server.Close()

	cfg := &Config{
		ClientID: "client-id",
		TokenURL: server.URL,
	}
	client := NewClient(cfg)
	data, err := client.ExchangeCode("authcode123", "https://example.com/callback", "secret")
	if err != nil {
		t.Fatalf("ExchangeCode failed: %v", err)
	}
	if data["access_token"] != "token456" {
		t.Errorf("access_token mismatch: got %v", data["access_token"])
	}
	if data["refresh_token"] != "refresh789" {
		t.Errorf("refresh_token mismatch: got %v", data["refresh_token"])
	}
	if data["scope"] != "read" {
		t.Errorf("scope mismatch: got %v", data["scope"])
	}
}

func TestOAuthError(t *testing.T) {
	err := &OAuthError{Err: "invalid_request", Description: "missing parameter"}
	expectedMsg := "invalid_request: missing parameter"
	if err.Error() != expectedMsg {
		t.Errorf("Error() mismatch: got %q, want %q", err.Error(), expectedMsg)
	}
	err2 := &OAuthError{Err: "invalid_request"}
	if err2.Error() != "invalid_request" {
		t.Errorf("Error() without description mismatch: got %q", err2.Error())
	}
}