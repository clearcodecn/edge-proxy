package web

import (
	"log"
	"net/http"
	"runtime/debug"

	"edge-proxy/internal/config"
	"edge-proxy/internal/store"
)

type PageRenderer struct {
	tmpl      *Templates
	nodeName  string
	version   string
	adminUser string
}

func NewPageRenderer(t *Templates, nodeName, version, adminUser string) *PageRenderer {
	return &PageRenderer{tmpl: t, nodeName: nodeName, version: version, adminUser: adminUser}
}

// BuildVersion returns a short git revision string for the running binary,
// or "dev" when build info is unavailable (e.g. `go run`). Mirrors cmd_version.go.
func BuildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	rev := "dev"
	dirty := ""
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			if len(s.Value) > 8 {
				rev = s.Value[:8]
			} else {
				rev = s.Value
			}
		}
		if s.Key == "vcs.modified" && s.Value == "true" {
			dirty = "-dirty"
		}
	}
	return rev + dirty
}

func (p *PageRenderer) base(title, page string, authenticated bool) map[string]any {
	return map[string]any{
		"Title":         title,
		"Page":          page,
		"Authenticated": authenticated,
		"NodeName":      p.nodeName,
		"Version":       p.version,
		"User":          p.adminUser,
	}
}

func (p *PageRenderer) RenderLogin(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := p.base("登录", "login", false)
	data["Error"] = errMsg
	if err := p.tmpl.Login.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("[web] render login: %v", err)
	}
}

// DomainListView is the data passed to both the full domains page and the
// swappable list partial. Constructed by the route handler; consumed by the
// templates `domains.html` and the partial `{{define "domain_list"}}`.
type DomainListView struct {
	Items      []store.Domain
	Total      int
	Page       int
	PageSize   int
	TotalPages int
	Hosts      []string // chip prefill (echoed from query)
	HostsStr   string   // newline-joined value for the hidden form input
	Status     string   // "" means "all"
	SearchMode bool     // len(Hosts) > 0 — pagination is disabled
	Truncated  bool     // chip count exceeded the 200-cap and was clipped
}

func (p *PageRenderer) RenderDomains(w http.ResponseWriter, view DomainListView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := p.base("域名", "domains", true)
	data["List"] = view
	if err := p.tmpl.Domains.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("[web] render domains: %v", err)
	}
}

// RenderDomainList writes only the swappable partial (for htmx hx-target
// updates). The partial template is defined inside domains.html.
func (p *PageRenderer) RenderDomainList(w http.ResponseWriter, view DomainListView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.tmpl.Domains.ExecuteTemplate(w, "domain_list", view); err != nil {
		log.Printf("[web] render domain_list partial: %v", err)
	}
}

func (p *PageRenderer) RenderDomainRow(w http.ResponseWriter, d store.Domain) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.tmpl.Domains.ExecuteTemplate(w, "domain_row", d); err != nil {
		log.Printf("[web] render domain_row: %v", err)
	}
}

// UpstreamListView mirrors DomainListView for the upstream resource. Status is
// the string the UI sends ("enabled" / "disabled" / "" for all).
type UpstreamListView struct {
	Items      []store.Upstream
	Total      int
	Page       int
	PageSize   int
	TotalPages int
	Addrs      []string
	AddrsStr   string
	Status     string
	SearchMode bool
	Truncated  bool
}

func (p *PageRenderer) RenderUpstreams(w http.ResponseWriter, view UpstreamListView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := p.base("回源", "upstreams", true)
	data["List"] = view
	if err := p.tmpl.Upstreams.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("[web] render upstreams: %v", err)
	}
}

func (p *PageRenderer) RenderUpstreamList(w http.ResponseWriter, view UpstreamListView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := p.tmpl.Upstreams.ExecuteTemplate(w, "upstream_list", view); err != nil {
		log.Printf("[web] render upstream_list partial: %v", err)
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
	data := p.base("配置", "config", true)
	data["Admin"] = safe.Admin
	data["Acme"] = safe.Acme
	data["Probe"] = safe.Probe
	data["Alert"] = safe.Alert
	data["Paths"] = safe.Paths
	if err := p.tmpl.Config.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("[web] render config: %v", err)
	}
}
