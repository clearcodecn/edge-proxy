package alert

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegram_SendSuccess(t *testing.T) {
	var got struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewTelegram("bot-token", "-100123")
	c.Endpoint = srv.URL
	if err := c.Send(context.Background(), "edge-proxy hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.ChatID != "-100123" {
		t.Errorf("chat_id = %q", got.ChatID)
	}
	if got.Text != "edge-proxy hello" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestTelegram_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewTelegram("t", "c")
	c.Endpoint = srv.URL
	err := c.Send(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("expected 502 in error: %v", err)
	}
}

func TestTelegram_AppLevelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"bad chat id"}`))
	}))
	defer srv.Close()

	c := NewTelegram("t", "c")
	c.Endpoint = srv.URL
	err := c.Send(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bad chat id") {
		t.Errorf("expected description in error: %v", err)
	}
}

func TestTelegram_EmptyConfigErrors(t *testing.T) {
	c := NewTelegram("", "")
	if err := c.Send(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
}
