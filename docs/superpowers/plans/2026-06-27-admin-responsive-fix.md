# Admin Responsive Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the authenticated admin pages usable on both mobile and desktop by fixing table scrolling, aligning list-page forms, and introducing responsive navigation/layout behavior for domains, upstreams, and config.

**Architecture:** Keep the existing server-rendered Go template flow and daisyUI/Tailwind stack, but tighten container structure so horizontal scrolling works inside the table cards and navigation switches between desktop sidebar and mobile drawer. Use tests that assert key rendered HTML hooks before modifying templates, then regenerate the committed Tailwind output once new utility classes are in place.

**Tech Stack:** Go `net/http/httptest`, Go html/templates, Tailwind CSS, daisyUI, embedded static assets

---

## File Map

- Modify: `internal/web/handler/domain_test.go`
  - Add failing assertions for responsive shell hooks and domain list/table/form structure.
- Modify: `internal/web/handler/upstream_test.go`
  - Add failing assertions for responsive shell hooks and upstream list/table/form structure.
- Modify: `internal/web/handler/config_test.go`
  - Add failing assertions for responsive grid and mobile-friendly key/value layout markers.
- Modify: `internal/web/template/layout.html`
  - Implement desktop sidebar + mobile drawer shell and widen the main content container safely.
- Modify: `internal/web/template/domains.html`
  - Restructure the control bar into responsive sections and add a single reliable horizontal scroll wrapper around the table.
- Modify: `internal/web/template/upstreams.html`
  - Mirror the responsive structure used by the domains page.
- Modify: `internal/web/template/config.html`
  - Convert cards and `dl` blocks to responsive grid/alignment classes.
- Modify: `web/input.css`
  - Add small reusable component classes only if repeated utility groups become unreadable.
- Modify: `internal/web/static/tailwind.css`
  - Regenerate committed CSS artifact after template/class changes.

### Task 1: Add failing responsive layout tests

**Files:**
- Modify: `internal/web/handler/domain_test.go`
- Modify: `internal/web/handler/upstream_test.go`
- Modify: `internal/web/handler/config_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestDomain_ListGET_ResponsiveLayoutHooks(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	_, _ = repo.Create("mobile-a.com")

	rec := httptest.NewRecorder()
	h.ListGET(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()

	for _, want := range []string{
		`data-mobile-nav="admin"`,
		`data-admin-shell="responsive"`,
		`data-list-controls="domains"`,
		`data-list-table-scroll="domains"`,
		`class="table table-sm min-w-[720px]"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body:\n%s", want, body)
		}
	}
}

func TestUpstream_ListGET_ResponsiveLayoutHooks(t *testing.T) {
	h, repo, _ := newUpstreamHandler(t)
	_, _ = repo.Create(store.UpstreamInput{Addr: "10.0.0.5:80"})

	rec := httptest.NewRecorder()
	h.ListGET(rec, httptest.NewRequest("GET", "/upstreams", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()

	for _, want := range []string{
		`data-mobile-nav="admin"`,
		`data-admin-shell="responsive"`,
		`data-list-controls="upstreams"`,
		`data-list-table-scroll="upstreams"`,
		`class="table table-sm min-w-[880px]"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body:\n%s", want, body)
		}
	}
}

func TestConfig_GET_ResponsiveLayoutHooks(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{Bind: "127.0.0.1:8080", Username: "admin"},
		Acme:  config.AcmeConfig{Email: "ops@example.com", Directory: config.LetsEncryptProd},
		Paths: config.PathsConfig{
			DataDir:        "/var/lib/edge-proxy",
			NginxConfDir:   "/etc/nginx/conf.d",
			NginxReloadCmd: "systemctl reload nginx",
		},
		Probe: config.ProbeConfig{HealthPath: "/", TimeoutSeconds: 3, FailThreshold: 3, RecoverThreshold: 2},
	}

	h := NewConfigHandler(cfg)
	rec := httptest.NewRecorder()
	h.GET(rec, httptest.NewRequest("GET", "/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()

	for _, want := range []string{
		`data-mobile-nav="admin"`,
		`data-config-grid="responsive"`,
		`class="grid gap-4 md:grid-cols-2 xl:grid-cols-3"`,
		`class="grid gap-2 sm:grid-cols-[140px_minmax(0,1fr)]"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in body:\n%s", want, body)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/web/handler -run 'TestDomain_ListGET_ResponsiveLayoutHooks|TestUpstream_ListGET_ResponsiveLayoutHooks|TestConfig_GET_ResponsiveLayoutHooks' -count=1
```

Expected: FAIL because the new `data-*` hooks and responsive class strings are not present in the rendered HTML yet.

- [ ] **Step 3: Commit**

```bash
git add internal/web/handler/domain_test.go internal/web/handler/upstream_test.go internal/web/handler/config_test.go
git commit -m "test: cover admin responsive layout hooks"
```

### Task 2: Implement responsive shell in the shared layout

**Files:**
- Modify: `internal/web/template/layout.html`
- Test: `internal/web/handler/domain_test.go`
- Test: `internal/web/handler/upstream_test.go`
- Test: `internal/web/handler/config_test.go`

- [ ] **Step 1: Write the minimal layout implementation**

```html
{{if .Authenticated}}
  <div class="drawer lg:drawer-open" data-admin-shell="responsive">
    <input id="admin-nav-drawer" type="checkbox" class="drawer-toggle">
    <div class="drawer-content min-w-0">
      <div class="flex min-h-screen flex-col">
        <header class="navbar border-b border-base-300 bg-base-100 px-4 lg:px-6">
          <div class="flex items-center gap-2 lg:hidden">
            <label for="admin-nav-drawer" class="btn btn-ghost btn-sm" data-mobile-nav="admin" aria-label="打开导航">
              <span class="text-lg leading-none">☰</span>
            </label>
          </div>
          <div class="flex-1 min-w-0 flex items-baseline gap-3">
            <span class="truncate text-base font-semibold">{{.Title}}</span>
            <span class="hidden text-xs text-base-content/50 sm:inline">节点 · {{.NodeName}}</span>
          </div>
          <div class="flex items-center gap-2 sm:gap-3">
            <span class="hidden text-xs text-base-content/70 sm:inline">👤 {{.User}}</span>
            <form method="post" action="/logout">
              <button type="submit" class="btn btn-ghost btn-xs">退出</button>
            </form>
          </div>
        </header>
        <main class="flex-1 overflow-x-hidden p-4 lg:p-6">
          {{template "content" .}}
        </main>
      </div>
    </div>
    <div class="drawer-side z-40">
      <label for="admin-nav-drawer" class="drawer-overlay"></label>
      <aside class="min-h-full w-64 border-r border-base-300 bg-base-100">
        <!-- existing nav content -->
      </aside>
    </div>
  </div>
{{end}}
```

- [ ] **Step 2: Run tests to verify partial progress**

Run:

```bash
go test ./internal/web/handler -run 'TestDomain_ListGET_ResponsiveLayoutHooks|TestUpstream_ListGET_ResponsiveLayoutHooks|TestConfig_GET_ResponsiveLayoutHooks' -count=1
```

Expected: still FAIL, but failures should now be narrowed to page-specific hooks such as list controls, table wrappers, or config grid classes.

- [ ] **Step 3: Commit**

```bash
git add internal/web/template/layout.html
git commit -m "feat: add responsive admin shell"
```

### Task 3: Fix domains and upstreams responsive controls and table scrolling

**Files:**
- Modify: `internal/web/template/domains.html`
- Modify: `internal/web/template/upstreams.html`
- Test: `internal/web/handler/domain_test.go`
- Test: `internal/web/handler/upstream_test.go`

- [ ] **Step 1: Write the minimal template changes**

```html
<div x-data="pageState({resource:'domains'})" class="w-full space-y-4" @keydown.escape.window="closeModal()">
  <form ... class="card bg-base-100 shadow-sm" data-list-controls="domains">
    <div class="card-body gap-4 p-4">
      <div class="flex flex-col gap-3 xl:flex-row xl:items-start">
        <div class="min-w-0 flex-1">
          <!-- chip search -->
        </div>
        <div class="flex w-full flex-col gap-3 sm:flex-row xl:w-auto xl:flex-wrap xl:justify-end">
          <select name="status" class="select select-bordered select-sm w-full sm:w-44"></select>
          <div class="grid w-full gap-2 sm:grid-cols-[minmax(0,1fr)_auto] xl:w-auto">
            <form ... class="grid gap-2 sm:grid-cols-[minmax(0,220px)_auto] xl:grid-cols-[220px_auto]">
              <input ... class="input input-bordered input-sm w-full">
              <button ... class="btn btn-primary btn-sm w-full sm:w-auto">+ 新增</button>
            </form>
            <button ... class="btn btn-outline btn-sm w-full sm:w-auto">⬆ 批量导入</button>
          </div>
        </div>
      </div>
    </div>
  </form>

  <div class="card overflow-hidden bg-base-100 shadow-sm">
    <div class="card-body p-0">
      <div class="overflow-x-auto" data-list-table-scroll="domains">
        <table class="table table-sm min-w-[720px]">
```

```html
<div x-data="pageState({resource:'upstreams'})" class="w-full space-y-4" @keydown.escape.window="closeModal()">
  <form ... class="card bg-base-100 shadow-sm" data-list-controls="upstreams">
    <div class="card-body gap-4 p-4">
      <div class="flex flex-col gap-3 2xl:flex-row 2xl:items-start">
        <div class="min-w-0 flex-1">
          <!-- chip search -->
        </div>
        <div class="flex w-full flex-col gap-3 xl:w-auto">
          <select name="status" class="select select-bordered select-sm w-full sm:w-44"></select>
          <form ... class="grid gap-2 sm:grid-cols-2 xl:grid-cols-[180px_88px_180px_auto_auto]">
            <input name="addr" class="input input-bordered input-sm w-full">
            <input name="weight" class="input input-bordered input-sm w-full">
            <input name="remark" class="input input-bordered input-sm w-full">
            <label class="flex min-h-9 items-center gap-2 rounded-btn border border-base-300 px-3">
              <input name="is_backup" type="checkbox" class="checkbox checkbox-xs">
              <span class="text-sm">备用</span>
            </label>
            <button type="submit" class="btn btn-primary btn-sm w-full xl:w-auto">+ 新增</button>
          </form>
          <button ... class="btn btn-outline btn-sm w-full sm:w-auto">⬆ 批量导入</button>
        </div>
      </div>
    </div>
  </form>

  <div class="card overflow-hidden bg-base-100 shadow-sm">
    <div class="card-body p-0">
      <div class="overflow-x-auto" data-list-table-scroll="upstreams">
        <table class="table table-sm min-w-[880px]">
```

- [ ] **Step 2: Run tests to verify they pass**

Run:

```bash
go test ./internal/web/handler -run 'TestDomain_ListGET_ResponsiveLayoutHooks|TestUpstream_ListGET_ResponsiveLayoutHooks' -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/web/template/domains.html internal/web/template/upstreams.html
git commit -m "feat: make admin lists responsive"
```

### Task 4: Fix config page responsive card and key/value layout

**Files:**
- Modify: `internal/web/template/config.html`
- Test: `internal/web/handler/config_test.go`

- [ ] **Step 1: Write the minimal template changes**

```html
<div class="w-full max-w-6xl space-y-4">
  <div role="alert" class="alert alert-info text-sm">
    ...
  </div>
  <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-3" data-config-grid="responsive">
    <div class="card bg-base-100 shadow-sm">
      <div class="card-body p-5">
        <h2 class="card-title text-base mb-1">管理</h2>
        <dl class="grid gap-2 text-sm sm:grid-cols-[140px_minmax(0,1fr)]">
          <dt class="text-base-content/60">bind</dt>
          <dd class="min-w-0 break-all"><code class="font-mono">{{.Admin.Bind}}</code></dd>
```

For wider label sets:

```html
<dl class="grid gap-2 text-sm sm:grid-cols-[180px_minmax(0,1fr)]">
```

And for the paths card:

```html
<div class="card bg-base-100 shadow-sm md:col-span-2 xl:col-span-3">
```

- [ ] **Step 2: Run tests to verify they pass**

Run:

```bash
go test ./internal/web/handler -run 'TestConfig_HidesSensitiveFields|TestConfig_UnconfiguredAlertChannels|TestConfig_GET_ResponsiveLayoutHooks' -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/web/template/config.html
git commit -m "feat: make config page responsive"
```

### Task 5: Regenerate Tailwind output and run final verification

**Files:**
- Modify: `web/input.css`
- Modify: `internal/web/static/tailwind.css`
- Verify: `internal/web/template/layout.html`
- Verify: `internal/web/template/domains.html`
- Verify: `internal/web/template/upstreams.html`
- Verify: `internal/web/template/config.html`

- [ ] **Step 1: Add shared CSS only if needed**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer components {
  .admin-kv {
    @apply grid gap-2 text-sm sm:grid-cols-[140px_minmax(0,1fr)];
  }
}
```

If repeated utilities remain readable without component extraction, keep `web/input.css` unchanged and skip this addition.

- [ ] **Step 2: Regenerate the committed CSS artifact**

Run:

```bash
npm run build:css
```

Expected: command exits `0` and updates `internal/web/static/tailwind.css` if new classes were introduced.

- [ ] **Step 3: Run the focused handler test suite**

Run:

```bash
go test ./internal/web/handler -count=1
```

Expected: PASS.

- [ ] **Step 4: Run the full project test suite**

Run:

```bash
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 5: Review generated diff**

Run:

```bash
git diff -- internal/web/template/layout.html internal/web/template/domains.html internal/web/template/upstreams.html internal/web/template/config.html web/input.css internal/web/static/tailwind.css internal/web/handler/domain_test.go internal/web/handler/upstream_test.go internal/web/handler/config_test.go
```

Expected: diff only shows the responsive layout, test assertions, and any intentional Tailwind artifact changes.

- [ ] **Step 6: Commit**

```bash
git add internal/web/template/layout.html internal/web/template/domains.html internal/web/template/upstreams.html internal/web/template/config.html web/input.css internal/web/static/tailwind.css internal/web/handler/domain_test.go internal/web/handler/upstream_test.go internal/web/handler/config_test.go
git commit -m "feat: fix admin responsive layout"
```
