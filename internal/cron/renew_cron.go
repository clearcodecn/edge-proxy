package cron

import (
	"context"
	"log"
	"time"

	"edge-proxy/internal/alert"
	"github.com/robfig/cron/v3"
)

const RenewSchedule = "0 3 * * *" // every day at 03:00 (5-field crontab)

type DomainRenewRepo interface {
	UpdateCertAt(host string, t time.Time) error
}

type CertbotRenewer interface {
	Renew() (renewed []string, failed []string, err error)
}

type NginxReloader interface {
	Reload() error
}

type RenewCron struct {
	Repo     DomainRenewRepo
	Certbot  CertbotRenewer
	Nginx    NginxReloader
	Notifier AlertNotifier
	Schedule string
	Now      func() time.Time
}

func NewRenewCron(repo DomainRenewRepo, cb CertbotRenewer, nx NginxReloader, n AlertNotifier) *RenewCron {
	return &RenewCron{
		Repo:     repo,
		Certbot:  cb,
		Nginx:    nx,
		Notifier: n,
		Schedule: RenewSchedule,
		Now:      time.Now,
	}
}

// Run blocks until ctx is cancelled, executing Tick at the configured schedule.
func (c *RenewCron) Run(ctx context.Context) {
	cr := cron.New()
	if _, err := cr.AddFunc(c.Schedule, func() { c.Tick(ctx) }); err != nil {
		log.Printf("[cron/renew] schedule %q: %v", c.Schedule, err)
		return
	}
	cr.Start()
	<-ctx.Done()
	<-cr.Stop().Done()
}

// Tick runs one renewal pass. Safe to call directly from tests.
func (c *RenewCron) Tick(_ context.Context) {
	renewed, failed, runErr := c.Certbot.Renew()

	now := c.Now()
	for _, host := range renewed {
		if err := c.Repo.UpdateCertAt(host, now); err != nil {
			log.Printf("[cron/renew] UpdateCertAt(%s): %v", host, err)
		}
	}

	// Always reload nginx — even when nothing was renewed it's a cheap no-op.
	// Reload picks up any cert files that certbot rewrote in place.
	if err := c.Nginx.Reload(); err != nil {
		log.Printf("[cron/renew] reload: %v", err)
		c.Notifier.Alert("edge_nginx:renew", alert.FormatNginxFailure("renew-reload", err.Error(), now))
	}

	for _, host := range failed {
		errMsg := "certbot renew failed"
		if runErr != nil {
			errMsg = runErr.Error()
		}
		c.Notifier.Alert("edge_renew:"+host, alert.FormatRenewFailure(host, errMsg, now))
	}
}
