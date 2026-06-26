package handler

import (
	"fmt"
	"net/http"

	"edge-proxy/internal/config"
)

type ConfigHandler struct {
	Cfg *config.Config
}

func NewConfigHandler(cfg *config.Config) *ConfigHandler {
	return &ConfigHandler{Cfg: cfg}
}

// GET renders a read-only view of selected, non-sensitive fields.
// MUST NOT emit password_hash, dingtalk.secret, telegram.bot_token.
func (h *ConfigHandler) GET(w http.ResponseWriter, _ *http.Request) {
	c := h.Cfg
	body := fmt.Sprintf(`<dl>
<dt>admin.bind</dt><dd>%s</dd>
<dt>admin.username</dt><dd>%s</dd>
<dt>acme.email</dt><dd>%s</dd>
<dt>acme.directory</dt><dd>%s</dd>
<dt>probe.health_path</dt><dd>%s</dd>
<dt>probe.timeout_seconds</dt><dd>%d</dd>
<dt>probe.fail_threshold</dt><dd>%d</dd>
<dt>probe.recover_threshold</dt><dd>%d</dd>
<dt>alert.dedup_window_minutes</dt><dd>%d</dd>
<dt>alert.dingtalk</dt><dd>%s</dd>
<dt>alert.telegram</dt><dd>%s</dd>
<dt>paths.data_dir</dt><dd>%s</dd>
<dt>paths.nginx_conf_dir</dt><dd>%s</dd>
<dt>paths.nginx_reload_cmd</dt><dd>%s</dd>
</dl>`,
		c.Admin.Bind,
		c.Admin.Username,
		c.Acme.Email,
		c.Acme.Directory,
		c.Probe.HealthPath,
		c.Probe.TimeoutSeconds,
		c.Probe.FailThreshold,
		c.Probe.RecoverThreshold,
		c.Alert.DedupWindowMinutes,
		yesNo(c.Alert.Dingtalk.Webhook != ""),
		yesNo(c.Alert.Telegram.BotToken != "" && c.Alert.Telegram.ChatID != ""),
		c.Paths.DataDir,
		c.Paths.NginxConfDir,
		c.Paths.NginxReloadCmd,
	)
	writeHTML(w, http.StatusOK, body)
}

func yesNo(b bool) string {
	if b {
		return "已配置"
	}
	return "未配置"
}
