package cron

import (
	"context"
	"errors"
	"log"
	"time"

	"edge-proxy/internal/alert"
	"edge-proxy/internal/nginx"
	"edge-proxy/internal/store"
)

const (
	ACMETickInterval = 30 * time.Second
	FailureThreshold = 5
	FailureBackoff   = 1 * time.Hour
)

// DomainACMERepo is the minimal repository surface the cron needs.
// Defined as an interface for test injection.
type DomainACMERepo interface {
	PickUnready(now time.Time) (*store.Domain, error)
	UpdateStatus(id int64, status string) error
	MarkCertApplied(id int64) error
	MarkCertFailed(id int64, errMsg string) error
	SetNextRetryAt(id int64, t time.Time) error
	GetByID(id int64) (*store.Domain, error)
}

type CertbotApplier interface {
	Apply(host string) error
}

type NginxApplier interface {
	WriteAndApply(filename string, content []byte) error
}

type AlertNotifier interface {
	Alert(key, message string)
}

type ACMECron struct {
	Repo     DomainACMERepo
	Certbot  CertbotApplier
	Nginx    NginxApplier
	Notifier AlertNotifier
	Interval time.Duration
	Now      func() time.Time // injectable clock for tests
}

func NewACMECron(repo DomainACMERepo, cb CertbotApplier, nx NginxApplier, n AlertNotifier) *ACMECron {
	return &ACMECron{
		Repo:     repo,
		Certbot:  cb,
		Nginx:    nx,
		Notifier: n,
		Interval: ACMETickInterval,
		Now:      time.Now,
	}
}

// Run blocks on a ticker loop until ctx is cancelled.
func (c *ACMECron) Run(ctx context.Context) {
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	c.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.Tick(ctx)
		}
	}
}

// Tick picks one unready domain and drives one full apply attempt.
// Safe to call directly from tests.
func (c *ACMECron) Tick(_ context.Context) {
	d, err := c.Repo.PickUnready(c.Now())
	if errors.Is(err, store.ErrNotFound) {
		return
	}
	if err != nil {
		log.Printf("[cron/acme] PickUnready: %v", err)
		return
	}

	// Mark as in-flight so a future tick won't double-dispatch.
	if err := c.Repo.UpdateStatus(d.ID, store.StatusCertApplying); err != nil {
		log.Printf("[cron/acme] UpdateStatus(cert_applying): %v", err)
		return
	}

	if err := c.Certbot.Apply(d.Host); err != nil {
		c.handleApplyFailure(d, err)
		return
	}

	confName := nginx.FileNameDomain(d.Host)
	confContent := nginx.RenderDomain(d.Host)
	if err := c.Nginx.WriteAndApply(confName, confContent); err != nil {
		c.handleNginxFailure(d, err)
		return
	}

	if err := c.Repo.MarkCertApplied(d.ID); err != nil {
		log.Printf("[cron/acme] MarkCertApplied: %v", err)
	}
}

func (c *ACMECron) handleApplyFailure(d *store.Domain, applyErr error) {
	msg := applyErr.Error()
	if err := c.Repo.MarkCertFailed(d.ID, msg); err != nil {
		log.Printf("[cron/acme] MarkCertFailed: %v", err)
	}
	c.Notifier.Alert("edge_acme:"+d.Host, alert.FormatACMEFailure(d.Host, msg, c.Now()))
	c.maybeBackoff(d.ID)
}

func (c *ACMECron) handleNginxFailure(d *store.Domain, nxErr error) {
	msg := "nginx apply: " + nxErr.Error()
	if err := c.Repo.MarkCertFailed(d.ID, msg); err != nil {
		log.Printf("[cron/acme] MarkCertFailed (nginx): %v", err)
	}
	c.Notifier.Alert("edge_nginx:"+d.Host, alert.FormatNginxFailure(d.Host, nxErr.Error(), c.Now()))
}

func (c *ACMECron) maybeBackoff(id int64) {
	updated, err := c.Repo.GetByID(id)
	if err != nil || updated == nil {
		return
	}
	if updated.FailCount >= FailureThreshold {
		_ = c.Repo.SetNextRetryAt(id, c.Now().Add(FailureBackoff))
	}
}
