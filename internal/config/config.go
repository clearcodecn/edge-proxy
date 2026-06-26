package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	LetsEncryptProd       = "https://acme-v02.api.letsencrypt.org/directory"
	defaultAdminBind      = "127.0.0.1:8080"
	defaultChallengePort  = 5002
	defaultHealthPath     = "/"
	defaultTimeoutSec     = 3
	defaultFailThreshold  = 3
	defaultRecoverThresh  = 2
	defaultDedupWindowMin = 60
	defaultReloadCmd      = "systemctl reload nginx"
)

type Config struct {
	Admin AdminConfig `yaml:"admin"`
	Acme  AcmeConfig  `yaml:"acme"`
	Probe ProbeConfig `yaml:"probe"`
	Alert AlertConfig `yaml:"alert"`
	Paths PathsConfig `yaml:"paths"`
}

type AdminConfig struct {
	Bind         string `yaml:"bind"`
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

type AcmeConfig struct {
	Email         string `yaml:"email"`
	Directory     string `yaml:"directory"`
	ChallengePort int    `yaml:"challenge_port"`
}

type ProbeConfig struct {
	HealthPath       string `yaml:"health_path"`
	TimeoutSeconds   int    `yaml:"timeout_seconds"`
	FailThreshold    int    `yaml:"fail_threshold"`
	RecoverThreshold int    `yaml:"recover_threshold"`
}

type AlertConfig struct {
	DedupWindowMinutes int            `yaml:"dedup_window_minutes"`
	Dingtalk           DingtalkConfig `yaml:"dingtalk"`
	Telegram           TelegramConfig `yaml:"telegram"`
}

type DingtalkConfig struct {
	Webhook string `yaml:"webhook"`
	Secret  string `yaml:"secret"`
}

type TelegramConfig struct {
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

type PathsConfig struct {
	DataDir        string `yaml:"data_dir"`
	NginxConfDir   string `yaml:"nginx_conf_dir"`
	NginxReloadCmd string `yaml:"nginx_reload_cmd"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Admin.Bind == "" {
		c.Admin.Bind = defaultAdminBind
	}
	if c.Acme.Directory == "" {
		c.Acme.Directory = LetsEncryptProd
	}
	if c.Acme.ChallengePort == 0 {
		c.Acme.ChallengePort = defaultChallengePort
	}
	if c.Probe.HealthPath == "" {
		c.Probe.HealthPath = defaultHealthPath
	}
	if c.Probe.TimeoutSeconds == 0 {
		c.Probe.TimeoutSeconds = defaultTimeoutSec
	}
	if c.Probe.FailThreshold == 0 {
		c.Probe.FailThreshold = defaultFailThreshold
	}
	if c.Probe.RecoverThreshold == 0 {
		c.Probe.RecoverThreshold = defaultRecoverThresh
	}
	if c.Alert.DedupWindowMinutes == 0 {
		c.Alert.DedupWindowMinutes = defaultDedupWindowMin
	}
	if c.Paths.NginxReloadCmd == "" {
		c.Paths.NginxReloadCmd = defaultReloadCmd
	}
}

func (c *Config) validate() error {
	required := []struct {
		name  string
		value string
	}{
		{"admin.username", c.Admin.Username},
		{"admin.password_hash", c.Admin.PasswordHash},
		{"acme.email", c.Acme.Email},
		{"paths.data_dir", c.Paths.DataDir},
		{"paths.nginx_conf_dir", c.Paths.NginxConfDir},
	}
	for _, r := range required {
		if r.value == "" {
			return fmt.Errorf("missing required field: %s", r.name)
		}
	}
	return nil
}
