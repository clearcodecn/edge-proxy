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

func (h *UpstreamHandler) ListGET(w http.ResponseWriter, _ *http.Request) {
	list, err := h.Repo.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderUpstreamList(w, list)
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
