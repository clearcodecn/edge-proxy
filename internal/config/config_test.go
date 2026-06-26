package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTmp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	return path
}

func fullCfg() string {
	return `
admin:
  username: admin
  password_hash: $2a$10$abcdefghij
acme:
  email: ops@example.com
paths:
  data_dir: /var/lib/edge-proxy
  nginx_conf_dir: /etc/nginx/conf.d
`
}

func TestLoad_Success(t *testing.T) {
	path := writeTmp(t, fullCfg())
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Admin.Username != "admin" {
		t.Errorf("Admin.Username = %q", cfg.Admin.Username)
	}
	if cfg.Acme.Email != "ops@example.com" {
		t.Errorf("Acme.Email = %q", cfg.Acme.Email)
	}
	if cfg.Admin.Bind != defaultAdminBind {
		t.Errorf("Admin.Bind default missing, got %q", cfg.Admin.Bind)
	}
	if cfg.Probe.FailThreshold != defaultFailThreshold {
		t.Errorf("Probe.FailThreshold default missing, got %d", cfg.Probe.FailThreshold)
	}
	if cfg.Acme.Directory != LetsEncryptProd {
		t.Errorf("Acme.Directory default missing, got %q", cfg.Acme.Directory)
	}
}

func TestLoad_MissingUsername(t *testing.T) {
	content := `
admin:
  password_hash: x
acme:
  email: ops@example.com
paths:
  data_dir: /var/lib/edge-proxy
  nginx_conf_dir: /etc/nginx/conf.d
`
	path := writeTmp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "admin.username") {
		t.Errorf("error should mention admin.username: %v", err)
	}
}

func TestLoad_MissingAcmeEmail(t *testing.T) {
	content := `
admin:
  username: admin
  password_hash: x
paths:
  data_dir: /var/lib/edge-proxy
  nginx_conf_dir: /etc/nginx/conf.d
`
	path := writeTmp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "acme.email") {
		t.Errorf("error should mention acme.email: %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/no/such/path/config.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Errorf("error should mention read config: %v", err)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTmp(t, "::: not yaml :::")
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse yaml") {
		t.Errorf("error should mention parse yaml: %v", err)
	}
}

func TestApplyDefaults_PreservesExplicit(t *testing.T) {
	c := &Config{
		Admin: AdminConfig{Bind: "0.0.0.0:9090", Username: "u", PasswordHash: "p"},
		Acme:  AcmeConfig{Email: "e@x.com", ChallengePort: 9999},
		Paths: PathsConfig{DataDir: "/d", NginxConfDir: "/n", NginxReloadCmd: "/bin/true"},
	}
	c.applyDefaults()
	if c.Admin.Bind != "0.0.0.0:9090" {
		t.Errorf("Admin.Bind overwritten: %q", c.Admin.Bind)
	}
	if c.Acme.ChallengePort != 9999 {
		t.Errorf("ChallengePort overwritten: %d", c.Acme.ChallengePort)
	}
	if c.Paths.NginxReloadCmd != "/bin/true" {
		t.Errorf("NginxReloadCmd overwritten: %q", c.Paths.NginxReloadCmd)
	}
}
