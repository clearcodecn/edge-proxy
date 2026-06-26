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

func TestDingtalk_SendSuccess(t *testing.T) {
	var got struct {
		MsgType string `json:"msgtype"`
		Text    struct {
			Content string `json:"content"`
		} `json:"text"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	c := NewDingtalk(srv.URL, "")
	if err := c.Send(context.Background(), "edge-proxy test message"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.MsgType != "text" {
		t.Errorf("msgtype = %q", got.MsgType)
	}
	if !strings.Contains(got.Text.Content, "告警") {
		t.Errorf("expected keyword 告警 prepended, got %q", got.Text.Content)
	}
}

func TestDingtalk_KeywordNotDoubled(t *testing.T) {
	var got struct {
		Text struct {
			Content string `json:"content"`
		} `json:"text"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	c := NewDingtalk(srv.URL, "")
	_ = c.Send(context.Background(), "【edge-proxy 测试告警】already has keyword")
	if strings.Count(got.Text.Content, "告警") < 1 {
		t.Errorf("keyword stripped: %q", got.Text.Content)
	}
	// Should not have been prepended again, content starts with original prefix.
	if !strings.HasPrefix(got.Text.Content, "【edge-proxy 测试告警】") {
		t.Errorf("expected original prefix preserved: %q", got.Text.Content)
	}
}

func TestDingtalk_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewDingtalk(srv.URL, "")
	err := c.Send(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in error: %v", err)
	}
}

func TestDingtalk_AppLevelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":310000,"errmsg":"keyword missing"}`))
	}))
	defer srv.Close()

	c := NewDingtalk(srv.URL, "")
	err := c.Send(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "310000") {
		t.Errorf("expected errcode in error: %v", err)
	}
}

func TestDingtalk_EmptyWebhookErrors(t *testing.T) {
	c := NewDingtalk("", "")
	err := c.Send(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error for empty webhook")
	}
}
