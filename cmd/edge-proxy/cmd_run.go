package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"

	"edge-proxy/internal/acme"
	"edge-proxy/internal/alert"
	"edge-proxy/internal/config"
	"edge-proxy/internal/cron"
	"edge-proxy/internal/nginx"
	"edge-proxy/internal/probe"
	"edge-proxy/internal/store"
	"edge-proxy/internal/web"
	"edge-proxy/internal/web/handler"
	mw "edge-proxy/internal/web/middleware"
)

const defaultConfigPath = "/etc/edge-proxy/config.yaml"

func runCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the edge-proxy main process (HTTP admin + crons)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdgeProxy(configPath)
		},
	}
	cmd.Flags().StringVar(&configPath, "config", defaultConfigPath, "path to config.yaml")
	return cmd
}

func runEdgeProxy(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if err := os.MkdirAll(cfg.Paths.DataDir, 0755); err != nil {
		return fmt.Errorf("mkdir data dir: %w", err)
	}

	db, err := store.Open(filepath.Join(cfg.Paths.DataDir, "edge.db"))
	if err != nil {
		return fmt.Errorf("sqlite open: %w", err)
	}
	if err := store.Migrate(db); err != nil {
		return fmt.Errorf("sqlite migrate: %w", err)
	}

	domainRepo := store.NewDomainRepo(db)
	upstreamRepo := store.NewUpstreamRepo(db)
	dedupRepo := store.NewAlertDedupRepo(db)

	nx := nginx.New(cfg.Paths.NginxConfDir, cfg.Paths.NginxReloadCmd)
	cb := acme.New(cfg.Acme.Email)

	notifier := buildNotifier(cfg, dedupRepo)

	if err := ensureBootstrapConf(nx); err != nil {
		log.Printf("WARN bootstrap conf apply failed: %v", err)
	}

	templates := web.MustLoadTemplates()
	hostName, _ := os.Hostname()
	if hostName == "" {
		hostName = "unknown"
	}
	pages := web.NewPageRenderer(templates, hostName, web.BuildVersion(), cfg.Admin.Username)
	sessions := mw.NewSessionStore(mw.DefaultSessionTTL)

	login := handler.NewLoginHandler(cfg.Admin.Username, cfg.Admin.PasswordHash, sessions)
	login.Renderer = pageLoginRenderer{pages: pages}
	domainHdl := handler.NewDomainHandler(domainRepo, cb, nx)
	upstreamHdl := handler.NewUpstreamHandler(upstreamRepo, nx)

	r := buildRouter(cfg, pages, login, domainHdl, upstreamHdl, domainRepo, upstreamRepo, sessions)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	startCrons(ctx, cfg, domainRepo, cb, nx, notifier)

	srv := &http.Server{
		Addr:              cfg.Admin.Bind,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("edge-proxy admin listening on %s", cfg.Admin.Bind)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down…")
	shutdownCtx, sCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer sCancel()
	return srv.Shutdown(shutdownCtx)
}

func buildNotifier(cfg *config.Config, dedup *store.AlertDedupRepo) *alert.Notifier {
	var channels []alert.Channel
	if cfg.Alert.Dingtalk.Webhook != "" {
		channels = append(channels, alert.NewDingtalk(cfg.Alert.Dingtalk.Webhook, cfg.Alert.Dingtalk.Secret))
	}
	if cfg.Alert.Telegram.BotToken != "" && cfg.Alert.Telegram.ChatID != "" {
		channels = append(channels, alert.NewTelegram(cfg.Alert.Telegram.BotToken, cfg.Alert.Telegram.ChatID))
	}
	return alert.NewNotifier(dedup, time.Duration(cfg.Alert.DedupWindowMinutes)*time.Minute, channels...)
}

// ensureBootstrapConf writes /etc/nginx/conf.d/edge-bootstrap.conf when missing.
// First-time only — preserves any operator customisation.
func ensureBootstrapConf(nx *nginx.Util) error {
	if nx.Exists(nginx.FileNameBootstrap) {
		return nil
	}
	log.Printf("Writing bootstrap conf to %s", nx.Path(nginx.FileNameBootstrap))
	return nx.WriteAndApply(nginx.FileNameBootstrap, nginx.RenderBootstrap())
}

func startCrons(ctx context.Context, cfg *config.Config, repo *store.DomainRepo, cb *acme.Certbot, nx *nginx.Util, notifier *alert.Notifier) {
	acmeCron := cron.NewACMECron(repo, cb, nx, notifier)

	probeFn := func(ctx context.Context, host, path string) probe.ProbeResult {
		return probe.CheckHealthyHTTPS(ctx, host, path)
	}
	probeCron := cron.NewProbeCron(repo, probeFn, notifier, cfg.Probe.HealthPath)
	probeCron.FailThreshold = cfg.Probe.FailThreshold
	probeCron.RecoverThreshold = cfg.Probe.RecoverThreshold

	renewCron := cron.NewRenewCron(repo, cb, nx, notifier)

	go acmeCron.Run(ctx)
	go probeCron.Run(ctx)
	go renewCron.Run(ctx)
}

func buildRouter(cfg *config.Config, pages *web.PageRenderer, login *handler.LoginHandler,
	domainHdl *handler.DomainHandler, upstreamHdl *handler.UpstreamHandler,
	domainRepo *store.DomainRepo, upstreamRepo *store.UpstreamRepo,
	sessions *mw.SessionStore,
) http.Handler {
	r := chi.NewRouter()
	r.Handle("/static/*", web.StaticHandler())
	r.Get("/login", login.GET)
	r.Post("/login", login.POST)
	r.Post("/logout", login.LogoutPOST)

	r.Group(func(r chi.Router) {
		r.Use(mw.RequireAuth(sessions, "/login"))
		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			q := req.URL.Query()
			view, err := web.BuildDomainListView(domainRepo, q.Get("hosts"), q.Get("status"), q.Get("page"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if req.Header.Get("HX-Request") == "true" {
				pages.RenderDomainList(w, view)
			} else {
				pages.RenderDomains(w, view)
			}
		})
		r.Get("/upstreams", func(w http.ResponseWriter, req *http.Request) {
			q := req.URL.Query()
			view, err := web.BuildUpstreamListView(upstreamRepo, q.Get("addrs"), q.Get("status"), q.Get("page"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if req.Header.Get("HX-Request") == "true" {
				pages.RenderUpstreamList(w, view)
			} else {
				pages.RenderUpstreams(w, view)
			}
		})
		r.Get("/config", func(w http.ResponseWriter, _ *http.Request) {
			pages.RenderConfig(w, cfg)
		})

		// Mutations + htmx row fragments
		r.Post("/domains", domainHdl.CreatePOST)
		r.Post("/domains/{id}/deprecate", domainHdl.DeprecatePOST)
		r.Post("/domains/{id}/recycle", domainHdl.RecyclePOST)
		r.Post("/domains/{id}/retry", domainHdl.RetryPOST)

		// Domain batch endpoints (Phase 4 of redesign-admin-ui).
		r.Post("/domains/batch", domainHdl.BatchImportPOST)
		r.Post("/domains/batch/deprecate", domainHdl.BatchDeprecatePOST)
		r.Post("/domains/batch/retry", domainHdl.BatchRetryPOST)
		r.Post("/domains/batch/recycle", domainHdl.BatchRecyclePOST)

		r.Post("/upstreams", upstreamHdl.CreatePOST)
		r.Post("/upstreams/{id}/toggle", upstreamHdl.TogglePOST)
		r.Delete("/upstreams/{id}", upstreamHdl.DeleteHTTP)

		// Upstream batch endpoints.
		r.Post("/upstreams/batch", upstreamHdl.BatchImportPOST)
		r.Post("/upstreams/batch/enable", upstreamHdl.BatchEnablePOST)
		r.Post("/upstreams/batch/disable", upstreamHdl.BatchDisablePOST)
		r.Delete("/upstreams/batch", upstreamHdl.BatchDeletePOST)
	})

	return r
}

// pageLoginRenderer adapts web.PageRenderer to handler.LoginRenderer.
type pageLoginRenderer struct {
	pages *web.PageRenderer
}

func (p pageLoginRenderer) Render(w http.ResponseWriter, errMsg string) {
	p.pages.RenderLogin(w, errMsg)
}
