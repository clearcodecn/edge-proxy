package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type TelegramClient struct {
	BotToken string
	ChatID   string
	Endpoint string // override only for tests; default https://api.telegram.org/bot<token>/sendMessage
	Timeout  time.Duration
	HTTP     *http.Client
}

func NewTelegram(botToken, chatID string) *TelegramClient {
	return &TelegramClient{
		BotToken: botToken,
		ChatID:   chatID,
		Timeout:  3 * time.Second,
		HTTP:     &http.Client{Timeout: 3 * time.Second},
	}
}

func (t *TelegramClient) Name() string { return "telegram" }

func (t *TelegramClient) endpoint() string {
	if t.Endpoint != "" {
		return t.Endpoint
	}
	return fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
}

type telegramPayload struct {
	ChatID string `json:"chat_id"`
	Text   string `json:"text"`
}

type telegramResp struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
}

func (t *TelegramClient) Send(ctx context.Context, text string) error {
	if t.BotToken == "" || t.ChatID == "" {
		return errors.New("telegram bot_token / chat_id not configured")
	}
	body, err := json.Marshal(telegramPayload{ChatID: t.ChatID, Text: text})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	sendCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(sendCtx, http.MethodPost, t.endpoint(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("telegram http %d", resp.StatusCode)
	}
	var tr telegramResp
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !tr.OK {
		return fmt.Errorf("telegram error_code=%d description=%s", tr.ErrorCode, tr.Description)
	}
	return nil
}
