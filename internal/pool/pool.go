package pool

import (
	"context"
	"log/slog"
	"time"

	"github.com/harshalvk/cage/internal/metrics"
	"github.com/harshalvk/cage/internal/sandbox"
)

type TemplateConfig struct {
	Slug  string
	Image string
	Size  int
}

type Pool struct {
	sm        *sandbox.SandboxManager
	templates map[string]TemplateConfig
	warm      map[string]chan string   // slug -> channel of ready container IDs
	refill    map[string]chan struct{} // slug -> signal to top up immediately
}

func New(sm *sandbox.SandboxManager, templates []TemplateConfig) *Pool {
	p := &Pool{
		sm:        sm,
		templates: make(map[string]TemplateConfig),
		warm:      make(map[string]chan string),
		refill:    make(map[string]chan struct{}),
	}

	for _, t := range templates {
		p.templates[t.Slug] = t
		p.warm[t.Slug] = make(chan string, t.Size)
		p.refill[t.Slug] = make(chan struct{}, t.Size)
	}

	return p
}

// start launches one maintenance goroutine per template. blocks until ctx is cancelled
func (p *Pool) Start(ctx context.Context) {
	for slug, cfg := range p.templates {
		go p.maintain(ctx, slug, cfg)
	}
}

func (p *Pool) maintain(ctx context.Context, slug string, cfg TemplateConfig) {
	// inital fill - done sequentially and deliberately at startup, before
	// traffic arrives, so that first real requests don't pay the cold-start cost

	for i := 0; i < cfg.Size; i++ {
		p.spawnOne(ctx, slug, cfg.Image)
	}

	// safety-net ticker, in case a refill signal is ever missed or a warm
	// container dies silently between Take() calls
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.refill[slug]:
			p.topUp(ctx, slug, cfg)
		case <-ticker.C:
			p.topUp(ctx, slug, cfg)
		}
	}
}

func (p *Pool) topUp(ctx context.Context, slug string, cfg TemplateConfig) {
	for len(p.warm[slug]) < cfg.Size {
		p.spawnOne(ctx, slug, cfg.Image)
	}
}

func (p *Pool) spawnOne(ctx context.Context, slug, image string) {
	containerID, err := p.sm.CreateSandbox(ctx, image)
	if err != nil {
		slog.Error("pool: failed to warm container for template %q: %v", "template", slug, "error", err)
		return
	}

	select {
	case p.warm[slug] <- containerID:
	default:
		// pool was already full by the time we finished creating this one
		// (e.g. a race with the ticker) - dont' leak it, clean it up
		_ = p.sm.KillSandbox(ctx, containerID)
	}
}

/*
Take function returns a ready-to-use container id for the given template, or
(false) if the pool is currently empty and the called should cold-start
it also verifies the container is actually still alive before handing it
out, discarding any that died unexpectedly
*/
func (p *Pool) Take(ctx context.Context, slug string) (containerID string, ok bool) {
	ch, exists := p.warm[slug]
	if !exists {
		metrics.PoolMisses.Inc()
		return "", false
	}

	for {
		select {
		case id := <-ch:
			running, err := p.sm.IsRunning(ctx, id)
			if err != nil || !running {
				slog.Info("pool: discarding dead warm container %s for template %q", id, slug)
				continue // try the next one in the channel, if any
			}
			p.triggerRefill(slug)
			metrics.PoolHits.Inc()
			return id, true
		default:
			return "", false
		}
	}
}

func (p *Pool) triggerRefill(slug string) {
	select {
	case p.refill[slug] <- struct{}{}:
	default:
		// a refill already pending - no need to queue another signal
	}
}
