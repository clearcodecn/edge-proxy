package handler

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

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

func (h *DomainHandler) ListGET(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hosts := splitHosts(q.Get("hosts"))
	// Hard-cap host search at MaxBatchSize to bound DOM size; UI will surface a
	// truncation banner when the original chip count exceeded the cap.
	if len(hosts) > MaxBatchSize {
		hosts = hosts[:MaxBatchSize]
	}
	status := strings.TrimSpace(q.Get("status"))
	if status == "all" {
		status = ""
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	list, _, err := h.Repo.Search(hosts, status, page, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderDomainList(w, list)
}

// BatchImportResult is the JSON body returned by POST /domains/batch.
// `created` carries the new IDs (preserving server order); `skipped` lists
// already-existing hosts; `failed` lists the rejects with reasons. HTTP status
// is 200 even when failed is non-empty — the partial-success semantics let the
// front-end show a per-row report.
type BatchImportResult struct {
	Created []int64        `json:"created"`
	Skipped []string       `json:"skipped"`
	Failed  []FailedImport `json:"failed"`
}

type FailedImport struct {
	Host   string `json:"host"`
	Reason string `json:"reason"`
}

// BatchResult is the common shape for action-style batch endpoints
// (deprecate / retry / recycle / enable / disable / delete).
type BatchResult struct {
	Succeeded []int64            `json:"succeeded"`
	Failed    []store.FailedItem `json:"failed"`
}

func (h *DomainHandler) BatchImportPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	raw := r.PostForm.Get("hosts")
	hosts := splitHosts(raw)
	if len(hosts) == 0 {
		http.Error(w, "未提供任何域名", http.StatusBadRequest)
		return
	}
	if len(hosts) > MaxBatchSize {
		http.Error(w, "单次导入不能超过 200 条", http.StatusBadRequest)
		return
	}

	result := BatchImportResult{}
	for _, host := range hosts {
		if !isValidHost(host) {
			result.Failed = append(result.Failed, FailedImport{Host: host, Reason: "无效的域名格式"})
			continue
		}
		d, err := h.Repo.Create(host)
		if errors.Is(err, store.ErrDuplicate) {
			result.Skipped = append(result.Skipped, host)
			continue
		}
		if err != nil {
			result.Failed = append(result.Failed, FailedImport{Host: host, Reason: err.Error()})
			continue
		}
		result.Created = append(result.Created, d.ID)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *DomainHandler) BatchDeprecatePOST(w http.ResponseWriter, r *http.Request) {
	ids, err := parseIDs(r)
	if err != nil {
		http.Error(w, "ids 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(ids) == 0 {
		http.Error(w, "未提供 ids", http.StatusBadRequest)
		return
	}
	if len(ids) > MaxBatchSize {
		http.Error(w, "单次批量操作不能超过 200 条", http.StatusBadRequest)
		return
	}
	allowed := []string{
		store.StatusPending, store.StatusCertApplying, store.StatusCertFailed,
		store.StatusOnline, store.StatusDegraded,
	}
	succ, failed := h.Repo.BatchUpdateStatus(ids, store.StatusDeprecated, allowed)
	writeJSON(w, http.StatusOK, BatchResult{Succeeded: succ, Failed: failed})
}

func (h *DomainHandler) BatchRetryPOST(w http.ResponseWriter, r *http.Request) {
	ids, err := parseIDs(r)
	if err != nil {
		http.Error(w, "ids 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(ids) == 0 {
		http.Error(w, "未提供 ids", http.StatusBadRequest)
		return
	}
	if len(ids) > MaxBatchSize {
		http.Error(w, "单次批量操作不能超过 200 条", http.StatusBadRequest)
		return
	}
	succ, failed := h.Repo.BatchResetRetry(ids)
	writeJSON(w, http.StatusOK, BatchResult{Succeeded: succ, Failed: failed})
}

// recycleReloadThrottle bounds the rate of nginx reload calls inside a single
// batch recycle. 100ms keeps a 200-id batch under ~20s while still avoiding the
// pathological "reload N times back-to-back" load. Exported as var so tests can
// shrink it.
var recycleReloadThrottle = 100 * time.Millisecond

func (h *DomainHandler) BatchRecyclePOST(w http.ResponseWriter, r *http.Request) {
	ids, err := parseIDs(r)
	if err != nil {
		http.Error(w, "ids 解析失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(ids) == 0 {
		http.Error(w, "未提供 ids", http.StatusBadRequest)
		return
	}
	if len(ids) > MaxBatchSize {
		http.Error(w, "单次批量操作不能超过 200 条", http.StatusBadRequest)
		return
	}

	// Repo only deletes rows whose status is deprecated; others come back as failed.
	deleted, failed := h.Repo.BatchDelete(ids, []string{store.StatusDeprecated})

	succeeded := make([]int64, 0, len(deleted))
	for i, d := range deleted {
		confName := nginx.FileNameDomain(d.Host)
		if rmErr := h.Nginx.RemoveFile(confName); rmErr != nil {
			// Best-effort: log and keep going. Conf may already be missing for old rows.
			log.Printf("[web/batch-recycle] remove conf %s: %v", confName, rmErr)
		}
		if testErr := h.Nginx.TestConfig(); testErr != nil {
			log.Printf("[web/batch-recycle] nginx -t for %s: %v", d.Host, testErr)
			failed = append(failed, store.FailedItem{ID: d.ID, Reason: "nginx -t: " + testErr.Error()})
			continue
		}
		if rlErr := h.Nginx.Reload(); rlErr != nil {
			log.Printf("[web/batch-recycle] nginx reload for %s: %v", d.Host, rlErr)
			failed = append(failed, store.FailedItem{ID: d.ID, Reason: "nginx reload: " + rlErr.Error()})
			continue
		}
		succeeded = append(succeeded, d.ID)

		// Throttle between reloads, except after the last one.
		if i < len(deleted)-1 && recycleReloadThrottle > 0 {
			time.Sleep(recycleReloadThrottle)
		}

		// Async LE delete; mirrors single-row recycle's fire-and-forget pattern.
		host := d.Host
		go func() {
			if err := h.Certbot.Delete(host); err != nil {
				log.Printf("[web/batch-recycle] certbot delete %s: %v", host, err)
			}
		}()
	}

	writeJSON(w, http.StatusOK, BatchResult{Succeeded: succeeded, Failed: failed})
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
