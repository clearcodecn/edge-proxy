package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"edge-proxy/internal/config"
)

func TestConfig_HidesSensitiveFields(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{
			Bind:         "127.0.0.1:8080",
			Username:     "admin",
			PasswordHash: "$2a$10$verysecrethash",
		},
		Acme: config.AcmeConfig{Email: "ops@example.com", Directory: config.LetsEncryptProd},
		Alert: config.AlertConfig{
			DedupWindowMinutes: 60,
			Dingtalk: config.DingtalkConfig{
				Webhook: "https://oapi.dingtalk.com/robot/send?access_token=SENSITIVE_TOKEN_HERE",
				Secret:  "SECVERYSECRET",
			},
			Telegram: config.TelegramConfig{
				BotToken: "BOT_SECRET_TOKEN_HERE",
				ChatID:   "-100123",
			},
		},
		Paths: config.PathsConfig{
			DataDir:        "/var/lib/edge-proxy",
			NginxConfDir:   "/etc/nginx/conf.d",
			NginxReloadCmd: "systemctl reload nginx",
		},
		Probe: config.ProbeConfig{
			HealthPath:       "/",
			TimeoutSeconds:   3,
			FailThreshold:    3,
			RecoverThreshold: 2,
		},
	}

	h := NewConfigHandler(cfg)
	rec := httptest.NewRecorder()
	h.GET(rec, httptest.NewRequest("GET", "/config", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	body := rec.Body.String()

	for _, mustHide := range []string{
		"$2a$10$verysecrethash",
		"SENSITIVE_TOKEN_HERE",
		"SECVERYSECRET",
		"BOT_SECRET_TOKEN_HERE",
	} {
		if strings.Contains(body, mustHide) {
			t.Errorf("secret %q leaked into config view:\n%s", mustHide, body)
		}
	}

	for _, mustShow := range []string{
		"127.0.0.1:8080",
		"admin",
		"ops@example.com",
		"已配置", // dingtalk + telegram "configured" marker
	} {
		if !strings.Contains(body, mustShow) {
			t.Errorf("expected %q in config view:\n%s", mustShow, body)
		}
	}
}

func TestConfig_UnconfiguredAlertChannels(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{Bind: "x", Username: "y"},
		Paths: config.PathsConfig{DataDir: "x", NginxConfDir: "y"},
	}
	h := NewConfigHandler(cfg)
	rec := httptest.NewRecorder()
	h.GET(rec, httptest.NewRequest("GET", "/config", nil))
	if !strings.Contains(rec.Body.String(), "未配置") {
		t.Errorf("should show unconfigured marker:\n%s", rec.Body.String())
	}
}
