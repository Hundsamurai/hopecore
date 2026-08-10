package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	env := newTestEnv(t)

	rec := env.request(http.MethodGet, "/api/health", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d, ожидался %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("тело не парсится как JSON: %v (%s)", err, rec.Body.String())
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, ожидалось "ok"`, body["status"])
	}
}

func TestHealthRejectsPost(t *testing.T) {
	env := newTestEnv(t)

	rec := env.request(http.MethodPost, "/api/health", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("статус = %d, ожидался %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
