package alert

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"edge-proxy/internal/probe"
)

const (
	acmeTitle   = "【edge-proxy ACME 申请告警】"
	renewTitle  = "【edge-proxy 续签告警】"
	nginxTitle  = "【edge-proxy Nginx 告警】"
	probeTitle  = "【edge-proxy 触达探测告警】"
	commonTail  = "1 小时内同 key 不再告警。"
	commonTailG = "1 小时内此告警不重复。"
)

func FormatACMEFailure(host, errMsg string, now time.Time) string {
	return strings.Join([]string{
		acmeTitle,
		"域名: " + host,
		"错误: " + trim(errMsg, 800),
		"时间: " + now.Format("2006-01-02 15:04:05"),
		commonTail,
	}, "\n")
}

func FormatRenewFailure(host, errMsg string, now time.Time) string {
	return strings.Join([]string{
		renewTitle,
		"域名: " + host,
		"错误: " + trim(errMsg, 800),
		"时间: " + now.Format("2006-01-02 15:04:05"),
		commonTail,
	}, "\n")
}

// FormatNginxFailure renders the alert body for nginx -t / reload failures.
// scope is either a domain host (e.g. "a.com") or the literal "upstream".
func FormatNginxFailure(scope, errMsg string, now time.Time) string {
	if scope == "" {
		scope = "-"
	}
	tail := commonTail
	if scope == "-" {
		tail = commonTailG
	}
	return strings.Join([]string{
		nginxTitle,
		"范围: " + scope,
		"错误: " + trim(errMsg, 800),
		"时间: " + now.Format("2006-01-02 15:04:05"),
		tail,
	}, "\n")
}

func FormatProbeFailure(host string, r probe.ProbeResult, now time.Time) string {
	var errLine string
	switch {
	case r.StatusCode != 0 && !isHealthyStatus(r.StatusCode):
		errLine = fmt.Sprintf("HTTP 状态码 %d (期望 200/204/301/302)", r.StatusCode)
	case r.Err != nil && isTimeout(r.Err):
		errLine = "请求超时 (>3s)"
	case r.Err != nil && isTLSError(r.Err):
		errLine = "TLS 校验失败: " + r.Err.Error()
	case r.Err != nil && isDNSError(r.Err):
		errLine = "DNS 解析失败: " + r.Err.Error()
	case r.Err != nil:
		errLine = "连接失败: " + r.Err.Error()
	default:
		errLine = "未知错误"
	}
	return strings.Join([]string{
		probeTitle,
		"域名: " + host,
		"错误: " + errLine,
		"时间: " + now.Format("2006-01-02 15:04:05"),
		commonTail,
	}, "\n")
}

func isHealthyStatus(c int) bool {
	return c == 200 || c == 204 || c == 301 || c == 302
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if os.IsTimeout(err) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

func isDNSError(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	return strings.Contains(err.Error(), "no such host")
}

func isTLSError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "x509") || strings.Contains(s, "certificate") || strings.Contains(s, "tls:")
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…[truncated]"
}
