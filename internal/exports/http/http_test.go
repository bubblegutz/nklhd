package http

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJoinURL(t *testing.T) {
	tests := []struct {
		base, path string
		want       string
		wantErr    bool
	}{
		{"https://example.com", "foo", "https://example.com/foo", false},
		{"https://example.com/", "foo", "https://example.com/foo", false},
		{"https://example.com/api", "foo", "https://example.com/api/foo", false},
		{"https://example.com/api/", "/foo", "https://example.com/api/foo", false},
		{"https://example.com", "https://other.com/bar", "https://other.com/bar", false},
		{"", "foo", "", true},
		{"https://example.com", "", "https://example.com/", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s+%s", tt.base, tt.path), func(t *testing.T) {
			got, err := joinURL(tt.base, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("joinURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("joinURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBasicAuth(t *testing.T) {
	got := BasicAuth("alice", "secret")
	// base64("alice:secret") = "YWxpY2U6c2VjcmV0"
	want := "YWxpY2U6c2VjcmV0"
	if got != want {
		t.Errorf("BasicAuth() = %v, want %v", got, want)
	}
}

func TestSimpleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello"))
	}))
	defer server.Close()

	result, err := SimpleRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("SimpleRequest failed: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, result.Status)
	}
	if result.Body != "hello" {
		t.Errorf("expected body 'hello', got %q", result.Body)
	}
}

func TestClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("response"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	result, err := client.Do("GET", "/test", nil)
	if err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}
	if result.Status != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, result.Status)
	}
	if result.Body != "response" {
		t.Errorf("expected body 'response', got %q", result.Body)
	}
	if result.Headers["X-Custom"] != "value" {
		t.Errorf("expected header X-Custom=value, got %v", result.Headers["X-Custom"])
	}
}

func TestClientPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "post data" {
			t.Errorf("expected body 'post data', got %q", body)
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	body := strings.NewReader("post data")
	result, err := client.Do("POST", "/submit", &Options{Body: body})
	if err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}
	if result.Status != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, result.Status)
	}
	if result.Body != "ok" {
		t.Errorf("expected body 'ok', got %q", result.Body)
	}
}

func TestClientHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "foo" {
			t.Errorf("missing header X-Test")
		}
		w.Write([]byte("headers ok"))
	}))
	defer server.Close()

	client := NewClient(server.URL, nil)
	result, err := client.Do("GET", "/", &Options{
		Headers: map[string]string{"X-Test": "foo"},
	})
	if err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}
	if result.Body != "headers ok" {
		t.Errorf("unexpected body: %s", result.Body)
	}
}

func TestClientAuthBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "alice" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("authenticated"))
	}))
	defer server.Close()

	client := NewClient(server.URL, &Options{
		Auth: &Auth{Type: "basic", Username: "alice", Password: "secret"},
	})
	result, err := client.Do("GET", "/", nil)
	if err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, result.Status)
	}
	if result.Body != "authenticated" {
		t.Errorf("expected body 'authenticated', got %q", result.Body)
	}
}

func TestClientAuthBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer mytoken" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("token ok"))
	}))
	defer server.Close()

	client := NewClient(server.URL, &Options{
		Auth: &Auth{Type: "bearer", Token: "mytoken"},
	})
	result, err := client.Do("GET", "/", nil)
	if err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, result.Status)
	}
	if result.Body != "token ok" {
		t.Errorf("expected body 'token ok', got %q", result.Body)
	}
}

func TestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("too slow"))
	}))
	defer server.Close()

	client := NewClient(server.URL, &Options{Timeout: 50 * time.Millisecond})
	_, err := client.Do("GET", "/", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Any error is acceptable (timeout or deadline exceeded)
}

func TestClientTLSInsecure(t *testing.T) {
	// Create a TLS server with self-signed cert
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tls ok"))
	}))
	defer server.Close()

	// Without TLS config, should fail
	client := NewClient(server.URL, nil)
	_, err := client.Do("GET", "/", nil)
	if err == nil {
		t.Fatal("expected TLS error")
	}

	// With InsecureSkipVerify, should succeed
	client2 := NewClient(server.URL, &Options{
		TLS: &tls.Config{InsecureSkipVerify: true},
	})
	result, err := client2.Do("GET", "/", nil)
	if err != nil {
		t.Fatalf("client with insecure TLS failed: %v", err)
	}
	if result.Body != "tls ok" {
		t.Errorf("expected body 'tls ok', got %q", result.Body)
	}
}