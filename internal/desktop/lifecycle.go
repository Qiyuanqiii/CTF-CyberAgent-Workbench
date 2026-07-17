package desktop

import (
	"context"
	"sync"
)

// WindowRestorer is the only native window action used for a second-instance
// launch. No arguments, working directory, path, or environment value from the
// second process crosses this boundary.
type WindowRestorer interface {
	Unminimise(context.Context)
	Show(context.Context)
}

// Lifecycle owns the cancellable Wails lifecycle context and coalesces any
// second-instance signal that arrives before startup completes.
type Lifecycle struct {
	mu        sync.RWMutex
	restoreMu sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	restorer  WindowRestorer
	started   bool
	stopped   bool
	pending   bool
}

func NewLifecycle(restorer WindowRestorer) *Lifecycle {
	return &Lifecycle{restorer: restorer}
}

func (l *Lifecycle) Start(parent context.Context) {
	if l == nil || parent == nil || parent.Err() != nil {
		return
	}
	l.mu.Lock()
	if l.started || l.stopped {
		l.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	l.ctx = ctx
	l.cancel = cancel
	l.started = true
	pending := l.pending
	l.pending = false
	l.mu.Unlock()
	if pending {
		l.restore(ctx)
	}
}

func (l *Lifecycle) Stop() {
	if l == nil {
		return
	}
	l.restoreMu.Lock()
	defer l.restoreMu.Unlock()
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.stopped = true
	l.pending = false
	l.ctx = nil
	cancel := l.cancel
	l.cancel = nil
	l.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (l *Lifecycle) Context() context.Context {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	ctx := l.ctx
	stopped := l.stopped
	l.mu.RUnlock()
	if stopped || ctx == nil || ctx.Err() != nil {
		return nil
	}
	return ctx
}

func (l *Lifecycle) RequestRestore() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.stopped || l.restorer == nil {
		l.mu.Unlock()
		return
	}
	if !l.started {
		l.pending = true
		l.mu.Unlock()
		return
	}
	ctx := l.ctx
	l.mu.Unlock()
	l.restore(ctx)
}

func (l *Lifecycle) restore(ctx context.Context) {
	l.restoreMu.Lock()
	defer l.restoreMu.Unlock()
	l.mu.RLock()
	active := !l.stopped && l.started && l.ctx == ctx
	restorer := l.restorer
	l.mu.RUnlock()
	if !active || ctx == nil || ctx.Err() != nil || restorer == nil {
		return
	}
	restorer.Unminimise(ctx)
	if ctx.Err() != nil {
		return
	}
	restorer.Show(ctx)
}
