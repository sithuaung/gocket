package api

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sithuaung/gocket/internal/app"
	"github.com/sithuaung/gocket/internal/auth"
)

const authTimestampTolerance = 600 // seconds

func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed := len(allowedOrigins) == 0 // allow all if none configured
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if !allowed {
				slog.Warn("origin rejected", "security_event", "origin_rejected", "origin", origin, "remote_addr", r.RemoteAddr)
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}

			allowOrigin := origin
			if len(allowedOrigins) == 0 {
				allowOrigin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func AuthMiddleware(apps *app.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			appID := r.PathValue("appId")
			a, ok := apps.FindByID(appID)
			if !ok {
				slog.Warn("unknown app", "security_event", "auth_failed", "reason", "unknown_app", "app_id", appID, "remote_addr", r.RemoteAddr)
				http.Error(w, `{"error":"unknown app"}`, http.StatusUnauthorized)
				return
			}

			q := r.URL.Query()
			authKey := q.Get("auth_key")
			authTimestamp := q.Get("auth_timestamp")
			authVersion := q.Get("auth_version")
			authSignature := q.Get("auth_signature")

			if authKey != a.Key {
				slog.Warn("invalid auth_key", "security_event", "auth_failed", "reason", "invalid_key", "app_id", appID, "remote_addr", r.RemoteAddr)
				http.Error(w, `{"error":"invalid auth_key"}`, http.StatusUnauthorized)
				return
			}

			if authVersion != "1.0" {
				http.Error(w, `{"error":"unsupported auth_version"}`, http.StatusUnauthorized)
				return
			}

			// Check timestamp within tolerance
			ts, err := strconv.ParseInt(authTimestamp, 10, 64)
			if err != nil {
				http.Error(w, `{"error":"invalid auth_timestamp"}`, http.StatusUnauthorized)
				return
			}
			if math.Abs(float64(time.Now().Unix()-ts)) > authTimestampTolerance {
				slog.Warn("auth timestamp expired", "security_event", "auth_failed", "reason", "timestamp_expired", "app_id", appID, "remote_addr", r.RemoteAddr)
				http.Error(w, `{"error":"auth_timestamp expired"}`, http.StatusUnauthorized)
				return
			}

			// Read body for MD5 validation
			var bodyBytes []byte
			if r.Body != nil {
				bodyBytes, err = io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}

			// Validate body_md5 if present
			if bodyMD5 := q.Get("body_md5"); bodyMD5 != "" {
				hash := md5.Sum(bodyBytes)
				expectedMD5 := hex.EncodeToString(hash[:])
				if bodyMD5 != expectedMD5 {
					slog.Warn("invalid body_md5", "security_event", "auth_failed", "reason", "body_md5_mismatch", "app_id", appID, "remote_addr", r.RemoteAddr)
					http.Error(w, `{"error":"invalid body_md5"}`, http.StatusUnauthorized)
					return
				}
			}

			// Build string to sign
			// Method\nPath\nparams (sorted, excluding auth_signature)
			params := make([]string, 0)
			for key, vals := range q {
				if key == "auth_signature" {
					continue
				}
				for _, val := range vals {
					params = append(params, fmt.Sprintf("%s=%s", key, val))
				}
			}
			sort.Strings(params)

			stringToSign := fmt.Sprintf("%s\n%s\n%s",
				r.Method,
				r.URL.Path,
				strings.Join(params, "&"),
			)

			expected := auth.HmacSHA256Hex(a.Secret, stringToSign)
			if !auth.HmacEqual(expected, authSignature) {
				slog.Warn("invalid auth_signature", "security_event", "auth_failed", "reason", "signature_mismatch", "app_id", appID, "remote_addr", r.RemoteAddr)
				http.Error(w, `{"error":"invalid auth_signature"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
