package acme

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ExecFunc is the subprocess runner. Returns combined output for diagnostics.
type ExecFunc func(name string, args ...string) ([]byte, error)

func defaultExec(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type Certbot struct {
	Email string
	Exec  ExecFunc
}

func New(email string) *Certbot {
	return &Certbot{Email: email, Exec: defaultExec}
}

// Apply runs `certbot certonly --nginx -d <host> --email <email> --agree-tos --non-interactive`.
// Mirrors im-api's scripts/apply_domain.sh.
func (c *Certbot) Apply(host string) error {
	out, err := c.Exec("certbot",
		"certonly", "--nginx",
		"--non-interactive",
		"-d", host,
		"--email", c.Email,
		"--agree-tos",
	)
	if err != nil {
		return fmt.Errorf("certbot apply %s: %w\n%s", host, err, truncate(string(out), 1024))
	}
	return nil
}

var (
	// Modern format: "Renewing an existing certificate for X" then a subsequent
	// "Successfully received certificate." line in the same per-cert section.
	renewingPattern = regexp.MustCompile(`(?m)^\s*Renewing an existing certificate for (\S+)`)
	// Legacy format: "/etc/letsencrypt/live/X/fullchain.pem (success)"
	renewSuccessPath = regexp.MustCompile(`/etc/letsencrypt/live/([^/]+)/fullchain\.pem \(success\)`)
	// Failure marker emitted per-cert in renew output.
	renewFailedRe = regexp.MustCompile(`(?m)^\s*Failed renewing certificate for (\S+)`)
)

// Renew runs `certbot renew --quiet --no-self-upgrade` and parses stdout to
// extract which hosts were renewed and which failed.
//
// Returns (renewed, failed, err). err is non-nil if the subprocess errored OR
// if any host failed; caller may still use renewed list to update cert_at.
func (c *Certbot) Renew() (renewed []string, failed []string, err error) {
	out, runErr := c.Exec("certbot", "renew", "--quiet", "--no-self-upgrade")
	s := string(out)
	renewed = extractRenewed(s)
	failed = extractFailed(s)
	if runErr != nil {
		return renewed, failed, fmt.Errorf("certbot renew: %w\n%s", runErr, truncate(s, 1024))
	}
	if len(failed) > 0 {
		return renewed, failed, fmt.Errorf("certbot renew: %d host(s) failed: %s", len(failed), strings.Join(failed, ", "))
	}
	return renewed, failed, nil
}

func extractRenewed(s string) []string {
	var hosts []string
	seen := map[string]bool{}
	// Modern certbot output: each per-cert section starts with "Renewing an existing certificate for X".
	indices := renewingPattern.FindAllStringSubmatchIndex(s, -1)
	for i, idx := range indices {
		host := s[idx[2]:idx[3]]
		end := len(s)
		if i+1 < len(indices) {
			end = indices[i+1][0]
		}
		section := s[idx[0]:end]
		if strings.Contains(section, "Successfully received certificate.") {
			if !seen[host] {
				hosts = append(hosts, host)
				seen[host] = true
			}
		}
	}
	// Legacy certbot output.
	for _, m := range renewSuccessPath.FindAllStringSubmatch(s, -1) {
		if len(m) >= 2 && !seen[m[1]] {
			hosts = append(hosts, m[1])
			seen[m[1]] = true
		}
	}
	return hosts
}

func extractFailed(s string) []string {
	var hosts []string
	for _, m := range renewFailedRe.FindAllStringSubmatch(s, -1) {
		if len(m) >= 2 {
			hosts = append(hosts, m[1])
		}
	}
	return hosts
}

// Delete runs `certbot delete --cert-name <host> --non-interactive`.
func (c *Certbot) Delete(host string) error {
	out, err := c.Exec("certbot", "delete", "--cert-name", host, "--non-interactive")
	if err != nil {
		return fmt.Errorf("certbot delete %s: %w\n%s", host, err, truncate(string(out), 512))
	}
	return nil
}

// LiveCertPath returns the standard Let's Encrypt fullchain.pem path for a host.
func (c *Certbot) LiveCertPath(host string) string {
	return "/etc/letsencrypt/live/" + host + "/fullchain.pem"
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "...[truncated]"
}
