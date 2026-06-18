package docprocessing

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultLLMCallPermitLimit  = 64
	defaultLLMCallPermitTTLSec = 320
	llmCallPermitLimitEnv      = "DOC_PROCESS_LLM_MAX_INFLIGHT_PER_MODEL"
	llmCallPermitTTLSecondsEnv = "DOC_PROCESS_LLM_PERMIT_TTL_SEC"
	llmCallPermitOverridesEnv  = "DOC_PROCESS_LLM_MAX_INFLIGHT_OVERRIDES"
)

type permitTimer interface {
	Stop() bool
}

type llmCallPermit interface {
	Release()
}

type llmPermitController interface {
	Acquire(ctx context.Context, modelName string) (llmCallPermit, error)
}

type permitScheduleFunc func(d time.Duration, fn func()) permitTimer

type timeAfterPermitTimer struct {
	timer *time.Timer
}

func (t *timeAfterPermitTimer) Stop() bool {
	if t == nil || t.timer == nil {
		return false
	}
	return t.timer.Stop()
}

type llmCallControllerConfig struct {
	DefaultLimit int
	LeaseTTL     time.Duration
	Overrides    map[string]int
	Schedule     permitScheduleFunc
}

type llmCallController struct {
	defaultLimit int
	leaseTTL     time.Duration
	overrides    map[string]int
	schedule     permitScheduleFunc

	mu      sync.Mutex
	permits map[string]chan struct{}
}

type acquiredLLMPermit struct {
	releaseOnce sync.Once
	timer       permitTimer
	releaseFn   func()
}

func (p *acquiredLLMPermit) Release() {
	if p == nil {
		return
	}
	p.releaseOnce.Do(func() {
		if p.timer != nil {
			p.timer.Stop()
		}
		if p.releaseFn != nil {
			p.releaseFn()
		}
	})
}

func newLLMCallController(cfg llmCallControllerConfig) *llmCallController {
	defaultLimit := cfg.DefaultLimit
	if defaultLimit < 1 {
		defaultLimit = 1
	}
	leaseTTL := cfg.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultLLMCallPermitTTLSec * time.Second
	}
	schedule := cfg.Schedule
	if schedule == nil {
		schedule = func(d time.Duration, fn func()) permitTimer {
			return &timeAfterPermitTimer{timer: time.AfterFunc(d, fn)}
		}
	}
	overrides := make(map[string]int, len(cfg.Overrides))
	for modelName, limit := range cfg.Overrides {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || limit < 1 {
			continue
		}
		overrides[modelName] = limit
	}
	return &llmCallController{
		defaultLimit: defaultLimit,
		leaseTTL:     leaseTTL,
		overrides:    overrides,
		schedule:     schedule,
		permits:      make(map[string]chan struct{}),
	}
}

func (c *llmCallController) Acquire(ctx context.Context, modelName string) (llmCallPermit, error) {
	if c == nil {
		return &acquiredLLMPermit{}, nil
	}
	permitCh := c.permitChannel(modelName)
	select {
	case permitCh <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	permit := &acquiredLLMPermit{
		releaseFn: func() {
			select {
			case <-permitCh:
			default:
			}
		},
	}
	permit.timer = c.schedule(c.leaseTTL, permit.Release)
	return permit, nil
}

func (c *llmCallController) permitChannel(modelName string) chan struct{} {
	modelName = strings.TrimSpace(modelName)
	c.mu.Lock()
	defer c.mu.Unlock()

	if permitCh, ok := c.permits[modelName]; ok {
		return permitCh
	}
	limit := c.defaultLimit
	if modelName != "" {
		if override, ok := c.overrides[modelName]; ok && override > 0 {
			limit = override
		}
	}
	permitCh := make(chan struct{}, limit)
	c.permits[modelName] = permitCh
	return permitCh
}

var defaultDocProcessorLLMController = newLLMCallController(loadLLMCallControllerConfigFromEnv())

func loadLLMCallControllerConfigFromEnv() llmCallControllerConfig {
	return llmCallControllerConfig{
		DefaultLimit: envInt(llmCallPermitLimitEnv, defaultLLMCallPermitLimit, 1),
		LeaseTTL:     time.Duration(envInt(llmCallPermitTTLSecondsEnv, defaultLLMCallPermitTTLSec, 1)) * time.Second,
		Overrides:    parseLLMCallPermitOverrides(os.Getenv(llmCallPermitOverridesEnv)),
	}
}

func parseLLMCallPermitOverrides(raw string) map[string]int {
	overrides := map[string]int{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		modelName, limitText, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		modelName = strings.TrimSpace(modelName)
		limitText = strings.TrimSpace(limitText)
		limit, err := strconv.Atoi(limitText)
		if err != nil || limit < 1 || modelName == "" {
			continue
		}
		overrides[modelName] = limit
	}
	return overrides
}
