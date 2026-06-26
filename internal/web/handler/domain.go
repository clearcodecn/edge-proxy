package handler

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"edge-proxy/internal/nginx"
	"edge-proxy/internal/store"
)

type DomainCertbot interface {
	Delete(host string) error
}

type DomainNginx interface {
	RemoveFile(filename string) error
	TestConfig() error
	Reload() error
}

type DomainHandler struct {
	Repo    *store.DomainRepo
	Certbot DomainCertbot
	Nginx   DomainNginx
}

func NewDomainHandler(repo *store.DomainRepo, cb DomainCertbot, nx DomainNginx) *DomainHandler {
	return &DomainHandler{Repo: repo, Certbot: cb, Nginx: nx}
}

// Domain hostname pattern (RFC 1035-ish, no strict label-length check — operator can use
// punycoded values manually if needed).
var hostPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)+$`)

func isValidHost(s string) bool {
	if len(s) < 3 || len(s) > 253 {
		return false
	}
	return hostPattern.MatchString(s)
}

func (h *DomainHandler) ListGET(w http.ResponseWriter, _ *http.Request) {
	list, err := h.Repo.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDomainList(w, list)
}

func (h *DomainHandler) CreatePOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	host := strings.TrimSpace(r.PostForm.Get("host"))
	host = strings.ToLower(host)
	if !isValidHost(host) {
		http.Error(w, "无效的域名格式", http.StatusBadRequest)
		return
	}
	d, err := h.Repo.Create(host)
	if errors.Is(err, store.ErrDuplicate) {
		http.Error(w, "域名已存在", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDomainRow(w, d)
}

func (h *DomainHandler) DeprecatePOST(w http.ResponseWriter, r *http.Request) {
	id, ok := parseURLParamID(r)
	if !ok {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	d, err := h.lookup(w, id)
	if d == nil {
		return
	}
	if d.Status == store.StatusDeprecated {
		http.Error(w, "已废弃", http.StatusBadRequest)
		return
	}
	if err := h.Repo.UpdateStatus(id, store.StatusDeprecated); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	d.Status = store.StatusDeprecated
	renderDomainRow(w, d)
	_ = err
}

func (h *DomainHandler) RecyclePOST(w http.ResponseWriter, r *http.Request) {
	id, ok := parseURLParamID(r)
	if !ok {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	d, err := h.lookup(w, id)
	if d == nil {
		return
	}
	if d.Status != store.StatusDeprecated {
		http.Error(w, "只有已废弃域名可回收", http.StatusBadRequest)
		return
	}

	if err := h.Repo.Delete(id); err != nil {
		http.Error(w, "db delete: "+err.Error(), http.StatusInternalServerError)
		return
	}

	confName := nginx.FileNameDomain(d.Host)
	if rmErr := h.Nginx.RemoveFile(confName); rmErr != nil {
		log.Printf("[web/recycle] remove conf %s: %v", confName, rmErr)
	}
	if testErr := h.Nginx.TestConfig(); testErr == nil {
		_ = h.Nginx.Reload()
	} else {
		log.Printf("[web/recycle] nginx -t failed after conf removal: %v", testErr)
	}

	// Best-effort certbot delete in background; failures only logged.
	go func() {
		if err := h.Certbot.Delete(d.Host); err != nil {
			log.Printf("[web/recycle] certbot delete %s: %v", d.Host, err)
		}
	}()

	w.WriteHeader(http.StatusOK)
	_ = err
}

func (h *DomainHandler) RetryPOST(w http.ResponseWriter, r *http.Request) {
	id, ok := parseURLParamID(r)
	if !ok {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	d, err := h.lookup(w, id)
	if d == nil {
		return
	}
	if d.Status != store.StatusCertFailed {
		http.Error(w, "只能重试失败的申请", http.StatusBadRequest)
		return
	}
	if err := h.Repo.ResetRetry(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	d.Status = store.StatusPending
	d.FailCount = 0
	d.LastError = ""
	renderDomainRow(w, d)
	_ = err
}

func (h *DomainHandler) lookup(w http.ResponseWriter, id int64) (*store.Domain, error) {
	d, err := h.Repo.GetByID(id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "domain not found", http.StatusNotFound)
		return nil, err
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, err
	}
	return d, nil
}

func parseURLParamID(r *http.Request) (int64, bool) {
	s := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
