package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const (
	testAPIToken         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testPushoverAppToken = "app_token_12345"
	testPushoverUserKey  = "user_key_67890"
)

func newTestApp(upstreamURL string) *app {
	return &app{
		apiTokenDigest:   sha256.Sum256([]byte(testAPIToken)),
		pushoverAppToken: testPushoverAppToken,
		pushoverUserKey:  testPushoverUserKey,
		httpClient:       &http.Client{Timeout: clientTimeout},
		upstreamURL:      upstreamURL,
		logger:           slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		env := map[string]string{
			"API_TOKEN":          testAPIToken,
			"PUSHOVER_APP_TOKEN": testPushoverAppToken,
			"PUSHOVER_USER_KEY":  testPushoverUserKey,
		}
		cfg, err := loadConfig(func(k string) string { return env[k] })
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		expectedDigest := sha256.Sum256([]byte(testAPIToken))
		if cfg.apiTokenDigest != expectedDigest {
			t.Fatalf("expected digest %x, got %x", expectedDigest, cfg.apiTokenDigest)
		}
		if cfg.pushoverAppToken != testPushoverAppToken {
			t.Fatalf("expected app token %q, got %q", testPushoverAppToken, cfg.pushoverAppToken)
		}
		if cfg.pushoverUserKey != testPushoverUserKey {
			t.Fatalf("expected user key %q, got %q", testPushoverUserKey, cfg.pushoverUserKey)
		}
	})

	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "missing API_TOKEN",
			env: map[string]string{
				"PUSHOVER_APP_TOKEN": testPushoverAppToken,
				"PUSHOVER_USER_KEY":  testPushoverUserKey,
			},
		},
		{
			name: "API_TOKEN too short",
			env: map[string]string{
				"API_TOKEN":          "0123456789abcdef",
				"PUSHOVER_APP_TOKEN": testPushoverAppToken,
				"PUSHOVER_USER_KEY":  testPushoverUserKey,
			},
		},
		{
			name: "API_TOKEN too long",
			env: map[string]string{
				"API_TOKEN":          testAPIToken + "00",
				"PUSHOVER_APP_TOKEN": testPushoverAppToken,
				"PUSHOVER_USER_KEY":  testPushoverUserKey,
			},
		},
		{
			name: "API_TOKEN non-hex characters",
			env: map[string]string{
				"API_TOKEN":          strings.Repeat("z", 64),
				"PUSHOVER_APP_TOKEN": testPushoverAppToken,
				"PUSHOVER_USER_KEY":  testPushoverUserKey,
			},
		},
		{
			name: "missing PUSHOVER_APP_TOKEN",
			env: map[string]string{
				"API_TOKEN":         testAPIToken,
				"PUSHOVER_USER_KEY": testPushoverUserKey,
			},
		},
		{
			name: "empty PUSHOVER_APP_TOKEN",
			env: map[string]string{
				"API_TOKEN":          testAPIToken,
				"PUSHOVER_APP_TOKEN": "",
				"PUSHOVER_USER_KEY":  testPushoverUserKey,
			},
		},
		{
			name: "missing PUSHOVER_USER_KEY",
			env: map[string]string{
				"API_TOKEN":          testAPIToken,
				"PUSHOVER_APP_TOKEN": testPushoverAppToken,
			},
		},
		{
			name: "empty PUSHOVER_USER_KEY",
			env: map[string]string{
				"API_TOKEN":          testAPIToken,
				"PUSHOVER_APP_TOKEN": testPushoverAppToken,
				"PUSHOVER_USER_KEY":  "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfig(func(k string) string { return tt.env[k] })
			if err == nil {
				t.Fatalf("expected error for case %q, got nil", tt.name)
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	app := newTestApp("")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", rec.Body.String())
	}
}

func TestRoutingAndMethods(t *testing.T) {
	app := newTestApp("")
	handler := app.routes()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{"GET /messages method not allowed", http.MethodGet, "/messages", http.StatusMethodNotAllowed},
		{"PUT /messages method not allowed", http.MethodPut, "/messages", http.StatusMethodNotAllowed},
		{"POST /healthz method not allowed", http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{"GET /unknown path not found", http.MethodGet, "/unknown", http.StatusNotFound},
		{"POST /unknown path not found", http.MethodPost, "/unknown", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
		})
	}
}

func TestAuthentication(t *testing.T) {
	app := newTestApp("")
	handler := app.routes()

	tests := []struct {
		name           string
		headers        map[string][]string
		expectedStatus int
	}{
		{
			name:           "missing Authorization header",
			headers:        map[string][]string{},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "non-Bearer scheme",
			headers: map[string][]string{
				"Authorization": {"Basic dXNlcjpwYXNz"},
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Bearer without token",
			headers: map[string][]string{
				"Authorization": {"Bearer "},
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "wrong token value",
			headers: map[string][]string{
				"Authorization": {"Bearer ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"},
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "multiple Authorization headers",
			headers: map[string][]string{
				"Authorization": {
					"Bearer " + testAPIToken,
					"Bearer " + testAPIToken,
				},
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(`{"message":"hello"}`))
			for k, v := range tt.headers {
				req.Header[k] = v
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
			if tt.expectedStatus == http.StatusUnauthorized {
				if auth := rec.Header().Get("WWW-Authenticate"); auth != "Bearer" {
					t.Fatalf("expected WWW-Authenticate: Bearer, got %q", auth)
				}
			}
		})
	}
}

func TestMessageValidation(t *testing.T) {
	app := newTestApp("")
	handler := app.routes()

	valid1024Runes := strings.Repeat("a", 1024)
	invalid1025Runes := strings.Repeat("a", 1025)
	valid1024Unicode := strings.Repeat("日", 1024)
	invalid1025Unicode := strings.Repeat("日", 1025)
	valid512URL := "https://example.com/" + strings.Repeat("a", 512-len("https://example.com/"))
	invalid513URL := valid512URL + "a"
	valid100Title := strings.Repeat("t", 100)
	invalid101Title := strings.Repeat("t", 101)

	tests := []struct {
		name           string
		body           string
		contentType    string
		expectedStatus int
	}{
		{
			name:           "empty body",
			body:           "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "malformed JSON",
			body:           `{invalid-json}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "JSON array instead of object",
			body:           `["message"]`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "JSON number",
			body:           `12345`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unknown JSON field",
			body:           `{"message":"hello","extra":"field"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "trailing JSON object",
			body:           `{"message":"hello"} {"message":"again"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "trailing garbage text",
			body:           `{"message":"hello"} trailing text`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty message string",
			body:           `{"message":""}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "whitespace-only message",
			body:           `{"message":"   \t\n  "}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "message with 1025 ASCII runes",
			body:           `{"message":"` + invalid1025Runes + `"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "message with 1025 Unicode runes",
			body:           `{"message":"` + invalid1025Unicode + `"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty url string",
			body:           `{"message":"hello","url":""}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "whitespace-only url",
			body:           `{"message":"hello","url":"   \t  "}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "url exceeding 512 runes",
			body:           `{"message":"hello","url":"` + invalid513URL + `"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "url_title without url",
			body:           `{"message":"hello","url_title":"ArgoCD"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty url_title with valid url",
			body:           `{"message":"hello","url":"https://example.com","url_title":""}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "whitespace-only url_title with valid url",
			body:           `{"message":"hello","url":"https://example.com","url_title":"   "}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "url_title exceeding 100 runes",
			body:           `{"message":"hello","url":"https://example.com","url_title":"` + invalid101Title + `"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "timestamp zero",
			body:           `{"message":"hello","timestamp":0}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "timestamp negative",
			body:           `{"message":"hello","timestamp":-10}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "timestamp non-number",
			body:           `{"message":"hello","timestamp":"1718000000"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "null url",
			body:           `{"message":"ok","url":null}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "null url_title",
			body:           `{"message":"ok","url_title":null}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "null timestamp",
			body:           `{"message":"ok","timestamp":null}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "request body exceeding 8 KiB",
			body:           `{"message":"` + strings.Repeat("x", 8200) + `"}`,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+testAPIToken)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d (%s)", tt.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}

	// Verify valid payloads with custom Content-Type, limits, or optional fields
	validCases := []struct {
		name        string
		body        string
		contentType string
	}{
		{
			name:        "1024 ASCII runes",
			body:        `{"message":"` + valid1024Runes + `"}`,
			contentType: "application/json",
		},
		{
			name:        "1024 Unicode runes",
			body:        `{"message":"` + valid1024Unicode + `"}`,
			contentType: "text/plain",
		},
		{
			name:        "unspecified Content-Type",
			body:        `{"message":"valid content"}`,
			contentType: "",
		},
		{
			name:        "all optional fields valid at boundary limits",
			body:        `{"message":"deploy","url":"` + valid512URL + `","url_title":"` + valid100Title + `","timestamp":1718000000}`,
			contentType: "application/json",
		},
		{
			name:        "url only without url_title",
			body:        `{"message":"deploy","url":"https://example.com"}`,
			contentType: "application/json",
		},
		{
			name:        "timestamp only without url",
			body:        `{"message":"deploy","timestamp":1718000000}`,
			contentType: "application/json",
		},
	}

	for _, tt := range validCases {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]int{"status": 1})
			}))
			defer upstream.Close()

			appWithUpstream := newTestApp(upstream.URL)
			req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+testAPIToken)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rec := httptest.NewRecorder()
			appWithUpstream.routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("expected status 204, got %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUpstreamForwarding(t *testing.T) {
	t.Run("successful acceptance and queuing forwards original untrimmed message and credentials", func(t *testing.T) {
		messageWithSpaces := "  hello world with spaces  "
		var receivedBody []byte
		var receivedContentType string

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedContentType = r.Header.Get("Content-Type")
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 1, "request": "req-123"})
		}))
		defer upstream.Close()

		app := newTestApp(upstream.URL)
		body := `{"message":"` + messageWithSpaces + `"}`
		req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testAPIToken)
		rec := httptest.NewRecorder()

		app.routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d (%s)", rec.Code, rec.Body.String())
		}
		if receivedContentType != "application/x-www-form-urlencoded" {
			t.Fatalf("expected upstream Content-Type application/x-www-form-urlencoded, got %q", receivedContentType)
		}

		values, err := url.ParseQuery(string(receivedBody))
		if err != nil {
			t.Fatalf("failed to parse upstream form: %v", err)
		}
		if values.Get("token") != testPushoverAppToken {
			t.Fatalf("expected token %q, got %q", testPushoverAppToken, values.Get("token"))
		}
		if values.Get("user") != testPushoverUserKey {
			t.Fatalf("expected user %q, got %q", testPushoverUserKey, values.Get("user"))
		}
		if values.Get("message") != messageWithSpaces {
			t.Fatalf("expected untrimmed message %q, got %q", messageWithSpaces, values.Get("message"))
		}
		if values.Has("url") {
			t.Fatalf("expected no url field in upstream form, got %q", values.Get("url"))
		}
		if values.Has("url_title") {
			t.Fatalf("expected no url_title field in upstream form, got %q", values.Get("url_title"))
		}
		if values.Has("timestamp") {
			t.Fatalf("expected no timestamp field in upstream form, got %q", values.Get("timestamp"))
		}
	})

	t.Run("forwards optional url, url_title, and timestamp when provided", func(t *testing.T) {
		var receivedBody []byte

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 1})
		}))
		defer upstream.Close()

		app := newTestApp(upstream.URL)
		body := `{"message":"deploy ok","url":"https://argocd.internal","url_title":"ArgoCD UI","timestamp":1718000000}`
		req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+testAPIToken)
		rec := httptest.NewRecorder()

		app.routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("expected status 204, got %d (%s)", rec.Code, rec.Body.String())
		}

		values, err := url.ParseQuery(string(receivedBody))
		if err != nil {
			t.Fatalf("failed to parse upstream form: %v", err)
		}
		if values.Get("url") != "https://argocd.internal" {
			t.Fatalf("expected url %q, got %q", "https://argocd.internal", values.Get("url"))
		}
		if values.Get("url_title") != "ArgoCD UI" {
			t.Fatalf("expected url_title %q, got %q", "ArgoCD UI", values.Get("url_title"))
		}
		if values.Get("timestamp") != "1718000000" {
			t.Fatalf("expected timestamp %q, got %q", "1718000000", values.Get("timestamp"))
		}
	})

	tests := []struct {
		name               string
		upstreamStatusCode int
		upstreamBody       string
		expectedStatus     int
	}{
		{
			name:               "upstream status 0 failure",
			upstreamStatusCode: http.StatusOK,
			upstreamBody:       `{"status":0,"errors":["application token is invalid"]}`,
			expectedStatus:     http.StatusBadGateway,
		},
		{
			name:               "upstream non-200 status 400",
			upstreamStatusCode: http.StatusBadRequest,
			upstreamBody:       `{"status":0,"errors":["bad request"]}`,
			expectedStatus:     http.StatusBadGateway,
		},
		{
			name:               "upstream internal server error 500",
			upstreamStatusCode: http.StatusInternalServerError,
			upstreamBody:       `internal error`,
			expectedStatus:     http.StatusBadGateway,
		},
		{
			name:               "upstream invalid JSON response",
			upstreamStatusCode: http.StatusOK,
			upstreamBody:       `{not-valid-json}`,
			expectedStatus:     http.StatusBadGateway,
		},
		{
			name:               "upstream empty response body",
			upstreamStatusCode: http.StatusOK,
			upstreamBody:       ``,
			expectedStatus:     http.StatusBadGateway,
		},
		{
			name:               "upstream response exceeding 64 KiB",
			upstreamStatusCode: http.StatusOK,
			upstreamBody:       strings.Repeat("a", maxUpstreamBytes+1),
			expectedStatus:     http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.upstreamStatusCode)
				_, _ = w.Write([]byte(tt.upstreamBody))
			}))
			defer upstream.Close()

			app := newTestApp(upstream.URL)
			req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(`{"message":"test"}`))
			req.Header.Set("Authorization", "Bearer "+testAPIToken)
			rec := httptest.NewRecorder()

			app.routes().ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
			if !strings.Contains(rec.Body.String(), "upstream pushover error") {
				t.Fatalf("expected generic upstream error message, got: %q", rec.Body.String())
			}
		})
	}
}

type errorRoundTripper struct {
	err error
}

func (e *errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
}

type errorReadCloser struct {
	err error
}

func (e *errorReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e *errorReadCloser) Close() error {
	return nil
}

type responseRoundTripper struct {
	resp *http.Response
}

func (r *responseRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return r.resp, nil
}

func TestContextCancellationAndTransportError(t *testing.T) {
	t.Run("upstream transport error", func(t *testing.T) {
		app := newTestApp("http://127.0.0.1:1")
		app.httpClient = &http.Client{
			Transport: &errorRoundTripper{err: context.DeadlineExceeded},
		}

		req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(`{"message":"test"}`))
		req.Header.Set("Authorization", "Bearer "+testAPIToken)
		rec := httptest.NewRecorder()

		app.routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected status 502, got %d", rec.Code)
		}
	})

	t.Run("canceled request context", func(t *testing.T) {
		app := newTestApp("http://127.0.0.1:1")
		app.httpClient = &http.Client{
			Transport: &errorRoundTripper{err: context.Canceled},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(`{"message":"test"}`)).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer "+testAPIToken)
		rec := httptest.NewRecorder()

		app.routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected status 502, got %d", rec.Code)
		}
	})

	t.Run("upstream response body read error", func(t *testing.T) {
		app := newTestApp("http://127.0.0.1:1")
		app.httpClient = &http.Client{
			Transport: &responseRoundTripper{
				resp: &http.Response{
					StatusCode: http.StatusOK,
					Body:       &errorReadCloser{err: io.ErrUnexpectedEOF},
				},
			},
		}

		req := httptest.NewRequest(http.MethodPost, "/messages", strings.NewReader(`{"message":"test"}`))
		req.Header.Set("Authorization", "Bearer "+testAPIToken)
		rec := httptest.NewRecorder()

		app.routes().ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected status 502, got %d", rec.Code)
		}
	})
}
