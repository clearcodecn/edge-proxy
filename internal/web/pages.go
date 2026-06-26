package web

import (
	"log"
	"net/http"

	"edge-proxy/internal/config"
	"edge-proxy/internal/store"
)

type PageRenderer struct {
	tmpl *Templates
}

func NewPageRenderer(t *Templates) *PageRenderer { return &PageRenderer{tmpl: t} }

func (p *PageRenderer) RenderLogin(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.tmpl.Login.ExecuteTemplate(w, "layout", map[string]any{
		"Title":         "登录",
		"Page":          "login",
		"Authenticated": false,
		"Error":         errMsg,
	}); err != nil {
		log.Printf("[web] render login: %v", err)
	}
}

func (p *PageRenderer) RenderDomains(w http.ResponseWriter, domains []store.Domain) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.tmpl.Domains.ExecuteTemplate(w, "layout", map[string]any{
		"Title":         "域名",
		"Page":          "domains",
		"Authenticated": true,
		"Domains":       domains,
	}); err != nil {
		log.Printf("[web] render domains: %v", err)
	}
}

func (p *PageRenderer) RenderDomainRow(w http.ResponseWriter, d store.Domain) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.tmpl.Domains.ExecuteTemplate(w, "domain_row", d); err != nil {
		log.Printf("[web] render domain_row: %v", err)
	}
}

func (p *PageRenderer) RenderUpstreams(w http.ResponseWriter, items []store.Upstream) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.tmpl.Upstreams.ExecuteTemplate(w, "layout", map[string]any{
		"Title":         "回源",
		"Page":          "upstreams",
		"Authenticated": true,
		"Upstreams":     items,
	}); err != nil {
		log.Printf("[web] render upstreams: %v", err)
	}
}

func (p *PageRenderer) RenderUpstreamRow(w http.ResponseWriter, u store.Upstream) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.tmpl.Upstreams.ExecuteTemplate(w, "upstream_row", u); err != nil {
		log.Printf("[web] render upstream_row: %v", err)
	}
}

// SafeConfigView strips secrets before rendering.
type SafeConfigView struct {
	Admin SafeAdmin
	Acme  config.AcmeConfig
	Probe config.ProbeConfig
	Alert SafeAlert
	Paths config.PathsConfig
}

type SafeAdmin struct {
	Bind     string
	Username string
}

type SafeAlert struct {
	DedupWindowMinutes int
	Dingtalk           SafeDingtalk
	Telegram           SafeTelegram
}

type SafeDingtalk struct{ Webhook string } // only presence-check
type SafeTelegram struct {
	BotToken string
	ChatID   string
}

func (p *PageRenderer) RenderConfig(w http.ResponseWriter, cfg *config.Config) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	safe := SafeConfigView{
		Admin: SafeAdmin{Bind: cfg.Admin.Bind, Username: cfg.Admin.Username},
		Acme:  config.AcmeConfig{Email: cfg.Acme.Email, Directory: cfg.Acme.Directory, ChallengePort: cfg.Acme.ChallengePort},
		Probe: cfg.Probe,
		Alert: SafeAlert{
			DedupWindowMinutes: cfg.Alert.DedupWindowMinutes,
			Dingtalk:           SafeDingtalk{Webhook: cfg.Alert.Dingtalk.Webhook},
			Telegram:           SafeTelegram{BotToken: cfg.Alert.Telegram.BotToken, ChatID: cfg.Alert.Telegram.ChatID},
		},
		Paths: cfg.Paths,
	}
	if err := p.tmpl.Config.ExecuteTemplate(w, "layout", map[string]any{
		"Title":         "配置",
		"Page":          "config",
		"Authenticated": true,
		"Admin":         safe.Admin,
		"Acme":          safe.Acme,
		"Probe":         safe.Probe,
		"Alert":         safe.Alert,
		"Paths":         safe.Paths,
	}); err != nil {
		log.Printf("[web] render config: %v", err)
	}
}
