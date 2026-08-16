package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	defaultAddr        = ":8080"
	defaultUpstreamURL = "https://api.pushover.net/1/messages.json"
	maxBodyBytes       = 8192
	maxMessageRunes    = 1024
	maxURLRunes        = 512
	maxURLTitleRunes   = 100
	maxUpstreamBytes   = 64 * 1024
	headerReadTimeout  = 5 * time.Second
	readTimeout        = 10 * time.Second
	idleTimeout        = 60 * time.Second
	clientTimeout      = 10 * time.Second
	shutdownTimeout    = 10 * time.Second
)

type config struct {
	apiTokenDigest   [32]byte
	pushoverAppToken string
	pushoverUserKey  string
}

func loadConfig(getenv func(string) string) (config, error) {
	apiToken := getenv("API_TOKEN")
	if len(apiToken) != 64 {
		return config{}, errors.New("API_TOKEN must be exactly 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(apiToken); err != nil {
		return config{}, errors.New("API_TOKEN must be exactly 64 hexadecimal characters")
	}

	pushoverAppToken := getenv("PUSHOVER_APP_TOKEN")
	if pushoverAppToken == "" {
		return config{}, errors.New("PUSHOVER_APP_TOKEN must not be empty")
	}

	pushoverUserKey := getenv("PUSHOVER_USER_KEY")
	if pushoverUserKey == "" {
		return config{}, errors.New("PUSHOVER_USER_KEY must not be empty")
	}

	return config{
		apiTokenDigest:   sha256.Sum256([]byte(apiToken)),
		pushoverAppToken: pushoverAppToken,
		pushoverUserKey:  pushoverUserKey,
	}, nil
}

type app struct {
	apiTokenDigest   [32]byte
	pushoverAppToken string
	pushoverUserKey  string
	httpClient       *http.Client
	upstreamURL      string
	logger           *slog.Logger
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /messages", a.handleMessages)
	mux.HandleFunc("GET /healthz", a.handleHealthz)
	return mux
}

func (a *app) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) authenticate(r *http.Request) bool {
	authHeaders := r.Header.Values("Authorization")
	if len(authHeaders) != 1 {
		return false
	}

	token, ok := strings.CutPrefix(authHeaders[0], "Bearer ")
	if !ok {
		return false
	}

	digest := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(digest[:], a.apiTokenDigest[:]) == 1
}

type optional[T any] struct {
	value   T
	present bool
}

func (o *optional[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return errors.New("null value is not allowed")
	}
	o.present = true
	return json.Unmarshal(data, &o.value)
}

type messageRequest struct {
	Message   string           `json:"message"`
	URL       optional[string] `json:"url"`
	URLTitle  optional[string] `json:"url_title"`
	Timestamp optional[int64]  `json:"timestamp"`
}

type pushoverResponse struct {
	Status int `json:"status"`
}

func (a *app) handleMessages(w http.ResponseWriter, r *http.Request) {
	if !a.authenticate(r) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var req messageRequest
	if err := decoder.Decode(&req); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var extra struct{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	if utf8.RuneCountInString(req.Message) > maxMessageRunes {
		http.Error(w, "message exceeds limit", http.StatusBadRequest)
		return
	}

	if req.URL.present {
		if strings.TrimSpace(req.URL.value) == "" {
			http.Error(w, "url is empty", http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(req.URL.value) > maxURLRunes {
			http.Error(w, "url exceeds limit", http.StatusBadRequest)
			return
		}
	}

	if req.URLTitle.present {
		if !req.URL.present {
			http.Error(w, "url_title requires url", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.URLTitle.value) == "" {
			http.Error(w, "url_title is empty", http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(req.URLTitle.value) > maxURLTitleRunes {
			http.Error(w, "url_title exceeds limit", http.StatusBadRequest)
			return
		}
	}

	if req.Timestamp.present && req.Timestamp.value <= 0 {
		http.Error(w, "invalid timestamp", http.StatusBadRequest)
		return
	}

	form := url.Values{}
	form.Set("token", a.pushoverAppToken)
	form.Set("user", a.pushoverUserKey)
	form.Set("message", req.Message)
	if req.URL.present {
		form.Set("url", req.URL.value)
	}
	if req.URLTitle.present {
		form.Set("url_title", req.URLTitle.value)
	}
	if req.Timestamp.present {
		form.Set("timestamp", strconv.FormatInt(req.Timestamp.value, 10))
	}

	upstreamReq, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodPost,
		a.upstreamURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		a.logger.Error("failed to create upstream request", "error", err)
		http.Error(w, "upstream pushover error", http.StatusBadGateway)
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(upstreamReq)
	if err != nil {
		a.logger.Error("upstream pushover request failed", "error", err)
		http.Error(w, "upstream pushover error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBytes+1))
	if err != nil {
		a.logger.Error("failed to read upstream pushover response", "error", err)
		http.Error(w, "upstream pushover error", http.StatusBadGateway)
		return
	}

	if len(body) > maxUpstreamBytes {
		a.logger.Error("upstream pushover response too large")
		http.Error(w, "upstream pushover error", http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		a.logger.Error("upstream pushover returned non-200 status", "status_code", resp.StatusCode)
		http.Error(w, "upstream pushover error", http.StatusBadGateway)
		return
	}

	var pushoverResp pushoverResponse
	if err := json.Unmarshal(body, &pushoverResp); err != nil {
		a.logger.Error("failed to decode upstream pushover response", "error", err)
		http.Error(w, "upstream pushover error", http.StatusBadGateway)
		return
	}

	if pushoverResp.Status != 1 {
		a.logger.Error("upstream pushover returned failure status", "status", pushoverResp.Status)
		http.Error(w, "upstream pushover error", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	appInstance := &app{
		apiTokenDigest:   cfg.apiTokenDigest,
		pushoverAppToken: cfg.pushoverAppToken,
		pushoverUserKey:  cfg.pushoverUserKey,
		httpClient:       &http.Client{Timeout: clientTimeout},
		upstreamURL:      defaultUpstreamURL,
		logger:           logger,
	}

	srv := &http.Server{
		Addr:              defaultAddr,
		Handler:           appInstance.routes(),
		ReadHeaderTimeout: headerReadTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown error", "error", err)
			os.Exit(1)
		}
		logger.Info("server shutdown complete")
	}
}
