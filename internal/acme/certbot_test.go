package acme

import (
	"errors"
	"strings"
	"testing"
)

func fakeExec(captured *[]string, out []byte, err error) ExecFunc {
	return func(name string, args ...string) ([]byte, error) {
		*captured = append(*captured, name+" "+strings.Join(args, " "))
		return out, err
	}
}

func TestApply_Success(t *testing.T) {
	c := New("ops@example.com")
	var calls []string
	c.Exec = fakeExec(&calls, []byte("Successfully received certificate."), nil)
	if err := c.Apply("foo.com"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %v", calls)
	}
	wants := []string{"certbot", "certonly", "--nginx", "-d foo.com", "--email ops@example.com", "--agree-tos", "--non-interactive"}
	for _, w := range wants {
		if !strings.Contains(calls[0], w) {
			t.Errorf("call missing %q\nfull: %s", w, calls[0])
		}
	}
}

func TestApply_FailureCarriesStderr(t *testing.T) {
	c := New("ops@example.com")
	stderr := "DNS problem: NXDOMAIN looking up A for nope.example.com" + strings.Repeat(" details", 200)
	var calls []string
	c.Exec = fakeExec(&calls, []byte(stderr), errors.New("exit 1"))
	err := c.Apply("nope.example.com")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "nope.example.com") {
		t.Errorf("error should mention host: %v", err)
	}
	if !strings.Contains(err.Error(), "DNS problem") {
		t.Errorf("error should include stderr: %v", err)
	}
}

func TestRenew_ParsesTwoSuccessfulHosts(t *testing.T) {
	stdout := `Processing /etc/letsencrypt/renewal/a.com.conf
- - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
Renewing an existing certificate for a.com

Successfully received certificate.
Certificate is saved at: /etc/letsencrypt/live/a.com/fullchain.pem

Processing /etc/letsencrypt/renewal/b.com.conf
Renewing an existing certificate for b.com
Successfully received certificate.
Certificate is saved at: /etc/letsencrypt/live/b.com/fullchain.pem
`
	c := New("ops@example.com")
	var calls []string
	c.Exec = fakeExec(&calls, []byte(stdout), nil)
	renewed, failed, err := c.Renew()
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if len(failed) != 0 {
		t.Errorf("failed = %v", failed)
	}
	if len(renewed) != 2 || renewed[0] != "a.com" || renewed[1] != "b.com" {
		t.Errorf("renewed = %v", renewed)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "renew --quiet --no-self-upgrade") {
		t.Errorf("calls = %v", calls)
	}
}

func TestRenew_LegacyFormat(t *testing.T) {
	stdout := `Congratulations, all renewals succeeded:
  /etc/letsencrypt/live/c.com/fullchain.pem (success)
  /etc/letsencrypt/live/d.com/fullchain.pem (success)
`
	c := New("ops@example.com")
	c.Exec = func(string, ...string) ([]byte, error) { return []byte(stdout), nil }
	renewed, _, err := c.Renew()
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if len(renewed) != 2 || renewed[0] != "c.com" || renewed[1] != "d.com" {
		t.Errorf("renewed = %v", renewed)
	}
}

func TestRenew_FailedHostReportedInError(t *testing.T) {
	stdout := `Processing /etc/letsencrypt/renewal/x.com.conf
Renewing an existing certificate for x.com

Attempting to renew cert (x.com) failed.
Failed renewing certificate for x.com
`
	c := New("ops@example.com")
	c.Exec = func(string, ...string) ([]byte, error) { return []byte(stdout), nil }
	renewed, failed, err := c.Renew()
	if err == nil {
		t.Fatal("expected error when host failed to renew")
	}
	if !strings.Contains(err.Error(), "x.com") {
		t.Errorf("error should mention failing host: %v", err)
	}
	if len(failed) != 1 || failed[0] != "x.com" {
		t.Errorf("failed = %v", failed)
	}
	if len(renewed) != 0 {
		t.Errorf("renewed should be empty when same host failed, got %v", renewed)
	}
}

func TestRenew_SubprocessError(t *testing.T) {
	c := New("ops@example.com")
	c.Exec = func(string, ...string) ([]byte, error) { return []byte("oops"), errors.New("permission denied") }
	_, _, err := c.Renew()
	if err == nil {
		t.Fatal("expected subprocess error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should wrap subprocess err: %v", err)
	}
}

func TestDelete_Success(t *testing.T) {
	c := New("x@y.com")
	var calls []string
	c.Exec = fakeExec(&calls, []byte("done"), nil)
	if err := c.Delete("gone.com"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "delete --cert-name gone.com --non-interactive") {
		t.Errorf("calls = %v", calls)
	}
}

func TestDelete_Failure(t *testing.T) {
	c := New("x@y.com")
	c.Exec = func(string, ...string) ([]byte, error) { return []byte("cert not found"), errors.New("exit 2") }
	err := c.Delete("missing.com")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cert not found") {
		t.Errorf("error should include stderr: %v", err)
	}
}

func TestLiveCertPath(t *testing.T) {
	c := New("x@y.com")
	if got := c.LiveCertPath("a.com"); got != "/etc/letsencrypt/live/a.com/fullchain.pem" {
		t.Errorf("got %q", got)
	}
}
