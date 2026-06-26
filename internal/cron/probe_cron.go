package cron

import (
	"context"
	"log"
	"sync"
	"time"

	"edge-proxy/internal/alert"
	"edge-proxy/internal/probe"
	"edge-proxy/internal/store"
)

const (
	ProbeTickInterval     = 60 * time.Second
	ProbeMaxConcurrency   = 20
	ProbeFailThreshold    = 3
	ProbeRecoverThreshold = 2
)

type DomainProbeRepo interface {
	ListByStatus(statuses ...string) ([]store.Domain, error)
	UpdateProbeResult(id int64, ok bool, failCount int, status string) error
}

// ProbeFn matches probe.CheckHealthyHTTPS signature; injectable for tests.
type ProbeFn func(ctx context.Context, host, path string) probe.ProbeResult

type ProbeCron struct {
	Repo             DomainProbeRepo
	Probe            ProbeFn
	Notifier         AlertNotifier
	HealthPath       string
	FailThreshold    int
	RecoverThreshold int
	Interval         time.Duration
	MaxConcurrency   int
	Now              func() time.Time
}

func NewProbeCron(repo DomainProbeRepo, p ProbeFn, n AlertNotifier, healthPath string) *ProbeCron {
	return &ProbeCron{
		Repo:             repo,
		Probe:            p,
		Notifier:         n,
		HealthPath:       healthPath,
		FailThreshold:    ProbeFailThreshold,
		RecoverThreshold: ProbeRecoverThreshold,
		Interval:         ProbeTickInterval,
		MaxConcurrency:   ProbeMaxConcurrency,
		Now:              time.Now,
	}
}

func (c *ProbeCron) Run(ctx context.Context) {
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

// Tick probes all online/degraded domains in parallel (bounded), then applies
// state-machine transitions per result.
func (c *ProbeCron) Tick(ctx context.Context) {
	domains, err := c.Repo.ListByStatus(store.StatusOnline, store.StatusDegraded)
	if err != nil {
		log.Printf("[cron/probe] ListByStatus: %v", err)
		return
	}
	sem := make(chan struct{}, c.MaxConcurrency)
	var wg sync.WaitGroup
	for _, d := range domains {
		d := d
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			r := c.Probe(ctx, d.Host, c.HealthPath)
			c.applyResult(d, r)
		}()
	}
	wg.Wait()
}

// applyResult uses the existing FailCount column to track streaks: the count
// always represents "consecutive events in the unexpected direction for the
// current state." Online stores failure streak; degraded stores recovery streak.
// Resets to 0 when a state transition fires or when the trend flips.
func (c *ProbeCron) applyResult(d store.Domain, r probe.ProbeResult) {
	success := r.OK
	newStatus := d.Status
	newFailCount := 0
	triggerAlert := false

	switch {
	case success && d.Status == store.StatusOnline:
		newFailCount = 0
	case success && d.Status == store.StatusDegraded:
		newFailCount = d.FailCount + 1
		if newFailCount >= c.RecoverThreshold {
			newStatus = store.StatusOnline
			newFailCount = 0
		}
	case !success && d.Status == store.StatusOnline:
		newFailCount = d.FailCount + 1
		if newFailCount >= c.FailThreshold {
			newStatus = store.StatusDegraded
			newFailCount = 0
			triggerAlert = true
		}
	case !success && d.Status == store.StatusDegraded:
		newFailCount = 0
	}

	if err := c.Repo.UpdateProbeResult(d.ID, success, newFailCount, newStatus); err != nil {
		log.Printf("[cron/probe] UpdateProbeResult(%s): %v", d.Host, err)
	}
	if triggerAlert {
		c.Notifier.Alert("edge_probe:"+d.Host, alert.FormatProbeFailure(d.Host, r, c.Now()))
	}
}
