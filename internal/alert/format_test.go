package alert

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"edge-proxy/internal/probe"
)

var fixedTime = time.Date(2026, 6, 26, 14, 30, 0, 0, time.UTC)

func TestFormatACMEFailure(t *testing.T) {
	got := FormatACMEFailure("a.com", "dns lookup failed", fixedTime)
	for _, want := range []string{
		"【edge-proxy ACME 申请告警】",
		"域名: a.com",
		"错误: dns lookup failed",
		"2026-06-26 14:30:00",
		"告警",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatRenewFailure(t *testing.T) {
	got := FormatRenewFailure("b.com", "rate limit", fixedTime)
	if !strings.Contains(got, "【edge-proxy 续签告警】") {
		t.Errorf("missing title:\n%s", got)
	}
	if !strings.Contains(got, "域名: b.com") {
		t.Errorf("missing host:\n%s", got)
	}
}

func TestFormatNginxFailure_WithHost(t *testing.T) {
	got := FormatNginxFailure("c.com", "nginx -t failed", fixedTime)
	if !strings.Contains(got, "范围: c.com") {
		t.Errorf("missing scope:\n%s", got)
	}
}

func TestFormatNginxFailure_UpstreamScope(t *testing.T) {
	got := FormatNginxFailure("upstream", "all servers disabled", fixedTime)
	if !strings.Contains(got, "范围: upstream") {
		t.Errorf("missing upstream scope:\n%s", got)
	}
}

func TestFormatProbeFailure_StatusCode(t *testing.T) {
	r := probe.ProbeResult{StatusCode: 502}
	got := FormatProbeFailure("d.com", r, fixedTime)
	if !strings.Contains(got, "HTTP 状态码 502") {
		t.Errorf("missing status code line:\n%s", got)
	}
	if !strings.Contains(got, "域名: d.com") {
		t.Errorf("missing host:\n%s", got)
	}
	if !strings.Contains(got, "告警") {
		t.Errorf("missing keyword:\n%s", got)
	}
}

func TestFormatProbeFailure_Timeout(t *testing.T) {
	r := probe.ProbeResult{Err: context.DeadlineExceeded}
	got := FormatProbeFailure("e.com", r, fixedTime)
	if !strings.Contains(got, "请求超时") {
		t.Errorf("missing timeout phrasing:\n%s", got)
	}
}

func TestFormatProbeFailure_TLS(t *testing.T) {
	r := probe.ProbeResult{Err: errors.New("tls: handshake failure")}
	got := FormatProbeFailure("f.com", r, fixedTime)
	if !strings.Contains(got, "TLS 校验失败") {
		t.Errorf("missing TLS phrasing:\n%s", got)
	}
}

func TestFormatProbeFailure_GenericConnError(t *testing.T) {
	r := probe.ProbeResult{Err: errors.New("connection refused")}
	got := FormatProbeFailure("g.com", r, fixedTime)
	if !strings.Contains(got, "连接失败") {
		t.Errorf("expected generic conn error:\n%s", got)
	}
}

func TestFormatProbeFailure_HealthyStatusNotTreatedAsError(t *testing.T) {
	// 301 is in the healthy set; should fall through to "未知错误" when no err.
	r := probe.ProbeResult{StatusCode: 301}
	got := FormatProbeFailure("h.com", r, fixedTime)
	if strings.Contains(got, "HTTP 状态码 301") {
		t.Errorf("301 should not be treated as failure status:\n%s", got)
	}
}

func TestTrim(t *testing.T) {
	if got := trim("hello", 10); got != "hello" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("a", 100)
	if got := trim(long, 10); !strings.HasSuffix(got, "…[truncated]") {
		t.Errorf("got %q", got)
	}
}
