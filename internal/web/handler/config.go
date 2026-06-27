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
	body := fmt.Sprintf(
		`<div data-admin-shell="responsive"><button data-mobile-nav="admin" type="button">menu</button><div class="w-full space-y-4"><div data-config-grid="responsive" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">%s%s%s%s<div class="card bg-base-100 shadow-sm md:col-span-2 xl:col-span-2"><div class="card-body p-5"><h2 class="card-title text-base mb-1">路径</h2><dl class="grid gap-2 text-sm sm:grid-cols-[180px_minmax(0,1fr)]">%s%s%s</dl></div></div></div></div></div>`,
		configCard("管理", `grid gap-2 sm:grid-cols-[140px_minmax(0,1fr)]`,
			configRow("admin.bind", fmt.Sprintf(`<code class="font-mono">%s</code>`, c.Admin.Bind)),
			configRow("admin.username", c.Admin.Username),
		),
		configCard("ACME", `grid gap-2 sm:grid-cols-[140px_minmax(0,1fr)] text-sm`,
			configRow("acme.email", c.Acme.Email),
			configRow("acme.directory", fmt.Sprintf(`<code class="font-mono text-xs">%s</code>`, c.Acme.Directory)),
		),
		configCard("探测", `grid gap-2 sm:grid-cols-[140px_minmax(0,1fr)] text-sm`,
			configRow("probe.health_path", fmt.Sprintf(`<code class="font-mono">%s</code>`, c.Probe.HealthPath)),
			configRow("probe.timeout_seconds", fmt.Sprintf("%d", c.Probe.TimeoutSeconds)),
			configRow("probe.fail_threshold", fmt.Sprintf("%d", c.Probe.FailThreshold)),
			configRow("probe.recover_threshold", fmt.Sprintf("%d", c.Probe.RecoverThreshold)),
		),
		configCard("告警", `grid gap-2 sm:grid-cols-[140px_minmax(0,1fr)] text-sm`,
			configRow("alert.dedup_window_minutes", fmt.Sprintf("%d", c.Alert.DedupWindowMinutes)),
			configRow("alert.dingtalk", yesNoBadge(c.Alert.Dingtalk.Webhook != "")),
			configRow("alert.telegram", yesNoBadge(c.Alert.Telegram.BotToken != "" && c.Alert.Telegram.ChatID != "")),
		),
		configRow("paths.data_dir", fmt.Sprintf(`<code class="font-mono">%s</code>`, c.Paths.DataDir)),
		configRow("paths.nginx_conf_dir", fmt.Sprintf(`<code class="font-mono">%s</code>`, c.Paths.NginxConfDir)),
		configRow("paths.nginx_reload_cmd", fmt.Sprintf(`<code class="font-mono">%s</code>`, c.Paths.NginxReloadCmd)),
	)
	writeHTML(w, http.StatusOK, body)
}

func yesNo(b bool) string {
	if b {
		return "已配置"
	}
	return "未配置"
}
