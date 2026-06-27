// edge-proxy admin UI — Alpine.js components.
//
// Three components carry the entire UI:
//
//   toastBus   — global toast queue, mounted once in layout.html.
//   chipInput  — chip-style search input with multi-paste, IME-safe commit
//                rules, overflow popover, and a hidden form input that htmx
//                serialises.
//   pageState  — per-page coordinator: row selection, batch modal, action
//                submission, list refresh after htmx swaps.
//
// Wire-up convention:
//   - Each <tr> row in a list carries  data-row-check  on its checkbox AND
//     calls  @change="toggleRow(id, host, $event.target.checked)".
//   - The list container has  id="domain-list" or "upstream-list"  so the
//     pageState can clear selection on htmx:afterSwap.
//   - The search form fires a synthetic  chipchange  event that htmx
//     listens to (hx-trigger="chipchange, ...").

document.addEventListener('alpine:init', () => {
  // ───────────────────────────────────────────────────────────── toastBus
  Alpine.data('toastBus', () => ({
    items: [],
    _seq: 0,
    add(msg, type = 'info', ttl = 3500) {
      const id = ++this._seq;
      this.items.push({ id, msg, type });
      setTimeout(() => {
        this.items = this.items.filter((t) => t.id !== id);
      }, ttl);
    },
  }));

  // ───────────────────────────────────────────────────────────── chipInput
  // Props:
  //   inputName    — name of the hidden <input> that holds chips.join('\n')
  //   initialChips — array of strings to start with (server-rendered)
  Alpine.data('chipInput', (opts = {}) => ({
    chips: Array.isArray(opts.initialChips) ? [...opts.initialChips] : [],
    buffer: '',
    composing: false,
    overflowCount: 0,
    popoverOpen: false,
    _ro: null,

    init() {
      this.$nextTick(() => this.recompute());
      if (typeof ResizeObserver !== 'undefined' && this.$refs.strip) {
        this._ro = new ResizeObserver(() => this.recompute());
        this._ro.observe(this.$refs.strip);
      }
      this.$watch('chips', () => {
        this.$nextTick(() => this.recompute());
        // Notify the enclosing form so htmx can re-query.
        const form = this.$root.closest('form');
        if (form) form.dispatchEvent(new CustomEvent('chipchange', { bubbles: true }));
      });
    },

    destroy() {
      if (this._ro) this._ro.disconnect();
    },

    // Measure how many chip elements stick out past the strip's right edge.
    // Reserve space at the right for the "+N" badge + the actual <input>.
    recompute() {
      const strip = this.$refs.strip;
      if (!strip) {
        this.overflowCount = 0;
        return;
      }
      const stripRight = strip.getBoundingClientRect().right;
      const reserve = 110; // ~ +N badge + min input area
      let overflow = 0;
      strip.querySelectorAll('[data-chip]').forEach((el) => {
        const r = el.getBoundingClientRect().right;
        if (r > stripRight - reserve) overflow++;
      });
      this.overflowCount = overflow;
    },

    // Key separators that commit the current buffer to a chip.
    _isSeparatorKey(k) {
      return k === 'Enter' || k === ',' || k === ' ' || k === 'Tab';
    },

    onKey(e) {
      if (this.composing) return; // CJK IME: never commit during composition.
      if (this._isSeparatorKey(e.key)) {
        const v = this.buffer.trim();
        if (v) {
          this.addChip(v);
          this.buffer = '';
          e.preventDefault();
        } else if (e.key === 'Enter' || e.key === 'Tab') {
          // Block form submission / focus loss on empty Enter/Tab.
          e.preventDefault();
        }
      } else if (e.key === 'Backspace' && this.buffer === '' && this.chips.length > 0) {
        this.chips = this.chips.slice(0, -1);
      }
    },

    onPaste(e) {
      const text = (e.clipboardData || window.clipboardData).getData('text');
      if (!text) return;
      // If the pasted text has any separator (newline, comma, tab, space),
      // intercept and split into multiple chips at once.
      if (/[\n,\t]/.test(text) || text.includes(' ')) {
        e.preventDefault();
        this.splitAndAdd(text);
      }
    },

    onCompositionStart() {
      this.composing = true;
    },
    onCompositionEnd() {
      this.composing = false;
      // Don't auto-commit after IME finishes; user will press Enter explicitly.
    },

    addChip(s) {
      const v = s.trim().toLowerCase();
      if (!v) return;
      if (this.chips.includes(v)) return;
      this.chips = [...this.chips, v];
    },

    splitAndAdd(text) {
      const parts = text.split(/[\n,\t ]+/);
      const next = [...this.chips];
      const seen = new Set(next);
      for (const p of parts) {
        const v = p.trim().toLowerCase();
        if (!v || seen.has(v)) continue;
        seen.add(v);
        next.push(v);
      }
      this.chips = next;
      this.buffer = '';
    },

    removeChip(idx) {
      this.chips = this.chips.filter((_, i) => i !== idx);
    },

    clearAll() {
      this.chips = [];
      this.buffer = '';
      this.popoverOpen = false;
    },

    focusInput() {
      this.$refs.input?.focus();
    },

    togglePopover() {
      this.popoverOpen = !this.popoverOpen;
    },

    closePopover() {
      this.popoverOpen = false;
    },
  }));

  // ───────────────────────────────────────────────────────────── pageState
  // Props:
  //   resource — 'domains' (default) or 'upstreams'
  Alpine.data('pageState', (opts = {}) => ({
    resource: opts.resource === 'upstreams' ? 'upstreams' : 'domains',
    selection: new Map(), // id -> host (or addr)
    selectionVersion: 0,  // Alpine doesn't observe Map mutations; bump to force re-eval
    modal: { open: false, kind: null, action: null, title: '', danger: false },
    importText: '',
    importResult: null,
    submitting: false,

    init() {
      // Clear selection whenever the list container is swapped (paginate, search, refresh).
      document.body.addEventListener('htmx:afterSwap', (e) => {
        const listId = this._listId();
        if (e.target && (e.target.id === listId || e.target.querySelector?.('#' + listId))) {
          this.clearSelection();
        }
      });
    },

    _listId() {
      return this.resource === 'upstreams' ? 'upstream-list' : 'domain-list';
    },
    _itemLabel() {
      return this.resource === 'upstreams' ? '回源' : '域名';
    },

    get selectionCount() {
      // Touch selectionVersion so Alpine re-evaluates whenever it bumps.
      // eslint-disable-next-line no-unused-expressions
      this.selectionVersion;
      return this.selection.size;
    },
    get selectedHosts() {
      // eslint-disable-next-line no-unused-expressions
      this.selectionVersion;
      return [...this.selection.values()];
    },

    isSelected(id) {
      // eslint-disable-next-line no-unused-expressions
      this.selectionVersion;
      return this.selection.has(Number(id));
    },

    toggleRow(id, host, checked) {
      const key = Number(id);
      if (checked) this.selection.set(key, host);
      else this.selection.delete(key);
      this.selectionVersion++;
    },

    // Select all rows currently rendered in the list container (visible page).
    // Called from the header-checkbox change handler with the inverse state.
    selectAllVisible(setTo) {
      const list = document.getElementById(this._listId());
      if (!list) return;
      list.querySelectorAll('[data-row-check]').forEach((cb) => {
        const id = Number(cb.getAttribute('data-row-id'));
        const host = cb.getAttribute('data-row-host') || '';
        if (setTo) {
          this.selection.set(id, host);
          cb.checked = true;
        } else {
          this.selection.delete(id);
          cb.checked = false;
        }
      });
      this.selectionVersion++;
    },

    clearSelection() {
      this.selection.clear();
      this.selectionVersion++;
      // After htmx swap the rows are gone, but the header checkbox + any
      // stragglers (e.g. a sticky header in a different swap region) need reset.
      document.querySelectorAll('[data-row-check], [data-select-all]').forEach((cb) => {
        cb.checked = false;
      });
    },

    async copyHosts() {
      const list = this.selectedHosts.filter(Boolean);
      if (list.length === 0) {
        window.toast?.('未选中任何条目', 'warning');
        return;
      }
      try {
        await navigator.clipboard.writeText(list.join('\n'));
        window.toast?.(`已复制 ${list.length} 个${this._itemLabel()}`, 'success');
      } catch (e) {
        window.toast?.(`复制失败：${e.message}`, 'error');
      }
    },

    openAction(action, title, opts = {}) {
      if (this.selectionCount === 0) {
        window.toast?.('请先选择条目', 'warning');
        return;
      }
      this.modal = { open: true, kind: 'action', action, title, danger: !!opts.danger };
    },

    openImport() {
      this.importText = '';
      this.importResult = null;
      this.modal = { open: true, kind: 'import', action: null, title: `批量导入${this._itemLabel()}`, danger: false };
    },

    closeModal() {
      this.modal.open = false;
      this.submitting = false;
    },

    async submitAction() {
      if (this.submitting) return;
      this.submitting = true;
      try {
        const action = this.modal.action;
        const { url, method } = this._actionEndpoint(action);
        const body = new URLSearchParams();
        for (const id of this.selection.keys()) body.append('ids', String(id));
        const resp = await fetch(url, {
          method,
          body,
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        });
        if (!resp.ok) {
          const t = await resp.text();
          window.toast?.(`请求失败 ${resp.status}: ${t}`, 'error');
          return;
        }
        const r = await resp.json();
        const succ = (r.succeeded || []).length;
        const fail = (r.failed || []).length;
        const type = fail === 0 ? 'success' : (succ === 0 ? 'error' : 'warning');
        window.toast?.(`${this.modal.title}：成功 ${succ}${fail ? ` · 失败 ${fail}` : ''}`, type);
        this.refreshList();
        this.clearSelection();
        this.closeModal();
      } catch (e) {
        window.toast?.(`请求异常：${e.message}`, 'error');
      } finally {
        this.submitting = false;
      }
    },

    _actionEndpoint(action) {
      // /upstreams/batch (DELETE) for upstream delete; everything else is POST.
      if (this.resource === 'upstreams' && action === 'delete') {
        return { url: '/upstreams/batch', method: 'DELETE' };
      }
      const base = this.resource === 'upstreams' ? '/upstreams/batch' : '/domains/batch';
      return { url: `${base}/${action}`, method: 'POST' };
    },

    async submitImport() {
      if (this.submitting) return;
      this.submitting = true;
      try {
        const url = this.resource === 'upstreams' ? '/upstreams/batch' : '/domains/batch';
        const fieldName = this.resource === 'upstreams' ? 'lines' : 'hosts';
        const body = new URLSearchParams();
        body.set(fieldName, this.importText);
        const resp = await fetch(url, {
          method: 'POST',
          body,
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        });
        if (!resp.ok) {
          const t = await resp.text();
          this.importResult = { error: t || `HTTP ${resp.status}` };
          return;
        }
        const r = await resp.json();
        this.importResult = r;
        const created = (r.created || []).length;
        const skipped = (r.skipped || []).length;
        const failed = (r.failed || []).length;
        const type = failed === 0 ? 'success' : (created === 0 ? 'error' : 'warning');
        window.toast?.(
          `导入完成：${created} 创建` +
            (skipped ? ` · ${skipped} 跳过` : '') +
            (failed ? ` · ${failed} 失败` : ''),
          type,
        );
        if (created > 0) this.refreshList();
      } catch (e) {
        this.importResult = { error: e.message };
      } finally {
        this.submitting = false;
      }
    },

    refreshList() {
      const el = document.getElementById(this._listId());
      if (el && window.htmx) {
        window.htmx.trigger(el, 'refresh');
      }
    },
  }));
});

// Global hook so non-Alpine code can fire toasts (e.g. inline hx-on:: handlers).
window.toast = (msg, type = 'info') => {
  const host = document.querySelector('[x-data="toastBus()"]');
  if (host && host._x_dataStack) {
    host._x_dataStack[0].add(msg, type);
  } else {
    console.log('[toast:' + type + ']', msg);
  }
};
