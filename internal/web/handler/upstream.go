package handler

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"edge-proxy/internal/nginx"
	"edge-proxy/internal/store"
)

type UpstreamNginx interface {
	WriteAndApply(filename string, content []byte) error
}

type UpstreamHandler struct {
	Repo  *store.UpstreamRepo
	Nginx UpstreamNginx
}

func NewUpstreamHandler(repo *store.UpstreamRepo, nx UpstreamNginx) *UpstreamHandler {
	return &UpstreamHandler{Repo: repo, Nginx: nx}
}

// Permissive addr pattern: "host:port" or "ip:port"; host allows letters/digits/.-.
var addrPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\.\-]*:[0-9]{1,5}$`)

func isValidAddr(s string) bool {
	if len(s) < 3 || len(s) > 255 {
		return false
	}
	return addrPattern.MatchString(s)
}

func (h *UpstreamHandler) ListGET(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	addrs := splitHosts(q.Get("addrs"))
	if len(addrs) > MaxBatchSize {
		addrs = addrs[:MaxBatchSize]
	}
	var enabledFilter *bool
	switch strings.TrimSpace(q.Get("status")) {
	case "enabled":
		t := true
		enabledFilter = &t
	case "disabled":
		f := false
		enabledFilter = &f
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	list, _, err := h.Repo.Search(addrs, enabledFilter, page, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderUpstreamList(w, list)
}

// FailedUpstreamImport carries a per-line rejection from POST /upstreams/batch.
// The original line text is echoed back so the UI can highlight it.
type FailedUpstreamImport struct {
	Line   string `json:"line"`
	Reason string `json:"reason"`
}

type UpstreamBatchImportResult struct {
	Created []int64                `json:"created"`
	Failed  []FailedUpstreamImport `json:"failed"`
}

func (h *UpstreamHandler) BatchImportPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	raw := r.PostForm.Get("lines")
	rawLines := strings.Split(raw, "\n")

	// Pre-filter blank lines and enforce the cap on actual content.
	var lines []string
	for _, ln := range rawLines {
		trimmed := strings.TrimSpace(ln)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	if len(lines) == 0 {
		http.Error(w, "未提供任何行", http.StatusBadRequest)
		return
	}
	if len(lines) > MaxBatchSize {
		http.Error(w, "单次导入不能超过 200 条", http.StatusBadRequest)
		return
	}

	result := UpstreamBatchImportResult{}
	for _, line := range lines {
		fields, err := ParseUpstreamLine(line)
		if err != nil {
			result.Failed = append(result.Failed, FailedUpstreamImport{Line: line, Reason: err.Error()})
			continue
		}
		if !isValidAddr(fields.Addr) {
			result.Failed = append(result.Failed, FailedUpstreamImport{Line: line, Reason: "无效的回源地址 (期望 host:port)"})
			continue
		}
		u, err := h.Repo.Create(store.UpstreamInput{
			Addr:     fields.Addr,
			Weight:   fields.Weight,
			IsBackup: fields.IsBackup,
			Remark:   fields.Remark,
		})
		if errors.Is(err, store.ErrDuplicate) {
			result.Failed = append(result.Failed, FailedUpstreamImport{Line: line, Reason: "回源地址已存在"})
			continue
		}
		if err != nil {
			result.Failed = append(result.Failed, FailedUpstreamImport{Line: line, Reason: err.Error()})
			continue
		}
		result.Created = append(result.Created, u.ID)
	}

	// Single nginx refresh after the whole batch (the upstream conf is one file,
	// listing all enabled upstreams). Failure is logged but does NOT roll back
	// the per-row Create calls — the rows exist; nginx just hasn't picked them
	// up yet, which the operator can fix manually.
	if len(result.Created) > 0 {
		if applyErr := h.refresh(); applyErr != nil {
			log.Printf("[web/upstream-batch] refresh after batch import: %v", applyErr)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *UpstreamHandler) BatchEnablePOST(w http.ResponseWriter, r *http.Request) {
	h.batchSetEnabled(w, r, true)
}

func (h *UpstreamHandler) BatchDisablePOST(w http.ResponseWriter, r *http.Request) {
	h.batchSetEnabled(w, r, false)
}

func (h *UpstreamHandler) batchSetEnabled(w http.ResponseWriter, r *http.Request, target bool) {
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
	succ, failed := h.Repo.BatchSetEnabled(ids, target)
	if len(succ) > 0 {
		if rfErr := h.refresh(); rfErr != nil {
			log.Printf("[web/upstream-batch] refresh after batch enable/disable: %v", rfErr)
		}
	}
	writeJSON(w, http.StatusOK, BatchResult{Succeeded: succ, Failed: failed})
}

func (h *UpstreamHandler) BatchDeletePOST(w http.ResponseWriter, r *http.Request) {
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
	deleted, failed := h.Repo.BatchDelete(ids)
	succeeded := make([]int64, 0, len(deleted))
	for _, u := range deleted {
		succeeded = append(succeeded, u.ID)
	}
	if len(succeeded) > 0 {
		if rfErr := h.refresh(); rfErr != nil {
			// Empty pool after the delete is typical (e.g. clearing all upstreams);
			// log but don't fail the request — the rows are gone either way.
			log.Printf("[web/upstream-batch] refresh after batch delete: %v", rfErr)
		}
	}
	writeJSON(w, http.StatusOK, BatchResult{Succeeded: succeeded, Failed: failed})
}

func (h *UpstreamHandler) CreatePOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	addr := strings.TrimSpace(r.PostForm.Get("addr"))
	if !isValidAddr(addr) {
		http.Error(w, "无效的回源地址 (期望 host:port)", http.StatusBadRequest)
		return
	}
	weight := 1
	if w := r.PostForm.Get("weight"); w != "" {
		if v, err := strconv.Atoi(w); err == nil && v > 0 {
			weight = v
		}
	}
	isBackup := r.PostForm.Get("is_backup") == "1" || r.PostForm.Get("is_backup") == "true"
	remark := strings.TrimSpace(r.PostForm.Get("remark"))

	u, err := h.Repo.Create(store.UpstreamInput{
		Addr:     addr,
		Weight:   weight,
		IsBackup: isBackup,
		Remark:   remark,
	})
	if errors.Is(err, store.ErrDuplicate) {
		http.Error(w, "回源地址已存在", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if applyErr := h.refresh(); applyErr != nil {
		log.Printf("[web/upstream] refresh after create: %v", applyErr)
		http.Error(w, "nginx 应用失败: "+applyErr.Error(), http.StatusInternalServerError)
		return
	}
	renderUpstreamRow(w, u)
}

func (h *UpstreamHandler) TogglePOST(w http.ResponseWriter, r *http.Request) {
	id, ok := parseURLParamID(r)
	if !ok {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.Repo.Toggle(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "upstream not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.refresh(); err != nil {
		log.Printf("[web/upstream] refresh after toggle: %v", err)
		http.Error(w, "nginx 应用失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	u, _ := h.Repo.GetByID(id)
	if u != nil {
		renderUpstreamRow(w, u)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *UpstreamHandler) DeleteHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := parseURLParamID(r)
	if !ok {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.Repo.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.refresh(); err != nil {
		// Tolerate: empty pool after delete is the typical reason; row is gone, alert via log.
		log.Printf("[web/upstream] refresh after delete: %v", err)
	}
	w.WriteHeader(http.StatusOK)
}

// refresh re-renders the upstream conf from the current enabled set and applies it.
// Returns the underlying error (caller decides whether to fail or tolerate).
func (h *UpstreamHandler) refresh() error {
	items, err := h.Repo.ListEnabled()
	if err != nil {
		return err
	}
	content, err := nginx.RenderUpstream(items)
	if err != nil {
		return err
	}
	return h.Nginx.WriteAndApply(nginx.FileNameUpstream, content)
}
