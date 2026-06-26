package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const dingTalkKeyword = "告警"

type DingtalkClient struct {
	Webhook string
	Secret  string // optional, currently unused (keyword "告警" suffices)
	Timeout time.Duration
	HTTP    *http.Client
}

func NewDingtalk(webhook, secret string) *DingtalkClient {
	return &DingtalkClient{
		Webhook: webhook,
		Secret:  secret,
		Timeout: 3 * time.Second,
		HTTP:    &http.Client{Timeout: 3 * time.Second},
	}
}

type dingTalkPayload struct {
	MsgType string          `json:"msgtype"`
	Text    dingTalkTextMsg `json:"text"`
}

type dingTalkTextMsg struct {
	Content string `json:"content"`
}

type dingTalkResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (d *DingtalkClient) Name() string { return "dingtalk" }

func (d *DingtalkClient) Send(ctx context.Context, text string) error {
	if d.Webhook == "" {
		return errors.New("dingtalk webhook not configured")
	}
	if !strings.Contains(text, dingTalkKeyword) {
		text = "【" + dingTalkKeyword + "】\n" + text
	}
	body, err := json.Marshal(dingTalkPayload{
		MsgType: "text",
		Text:    dingTalkTextMsg{Content: text},
	})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	sendCtx, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(sendCtx, http.MethodPost, d.Webhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("dingtalk http %d", resp.StatusCode)
	}
	var dr dingTalkResp
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if dr.ErrCode != 0 {
		return fmt.Errorf("dingtalk errcode=%d errmsg=%s", dr.ErrCode, dr.ErrMsg)
	}
	return nil
}
