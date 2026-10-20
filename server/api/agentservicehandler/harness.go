package agentservicehandler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// HarnessCapability is an optional feature a harness adapter may support.
// SemOS checks a service's required capabilities before a run starts and
// calls optional operations only when the adapter declares them.
type HarnessCapability string

const (
	CapabilityStreaming                HarnessCapability = "streaming"
	CapabilityCancellation             HarnessCapability = "cancellation"
	CapabilityInteractiveToolApproval  HarnessCapability = "interactive_tool_approval"
	CapabilityUsageReporting           HarnessCapability = "usage_reporting"
	CapabilityHistoryReplay            HarnessCapability = "history_replay"
	CapabilityContextCapacityReporting HarnessCapability = "context_capacity_reporting"
	CapabilityHistorySummary           HarnessCapability = "history_summary"
)

// DefaultHarness is the harness of profiles stored before profiles named one.
const DefaultHarness = "pi"

// defaultMaxRequestBytes bounds a run request when an adapter declares no
// limit of its own.
const defaultMaxRequestBytes = 480 * 1024

var (
	ErrHarnessNotRegistered     = errors.New("agent harness is not registered")
	ErrHarnessCapabilityMissing = errors.New("agent harness lacks a required capability")
	ErrHarnessProfileInvalid    = errors.New("agent harness cannot run this profile")
)

// HarnessProfileValidator is implemented by adapters that accept only some
// profile settings, such as a single model provider.
type HarnessProfileValidator interface {
	ValidateProfile(AgentServiceProfile) error
}

// HarnessDescriptor is what an adapter declares about itself.
type HarnessDescriptor struct {
	Name           string
	AdapterVersion string
	Capabilities   []HarnessCapability
	// MaxRequestBytes is the largest encoded AgentRunRequest the adapter
	// accepts; 0 means defaultMaxRequestBytes.
	MaxRequestBytes int
}

func (d HarnessDescriptor) Supports(capability HarnessCapability) bool {
	for _, declared := range d.Capabilities {
		if declared == capability {
			return true
		}
	}
	return false
}

func (d HarnessDescriptor) maxRequestBytes() int {
	if d.MaxRequestBytes > 0 {
		return d.MaxRequestBytes
	}
	return defaultMaxRequestBytes
}

// HarnessRegistry holds the adapters SemOS can run services on, by harness
// name. Service-to-harness selection stays server-side in the profile.
type HarnessRegistry struct {
	mu       sync.RWMutex
	adapters map[string]HarnessAdapter
}

func NewHarnessRegistry() *HarnessRegistry {
	return &HarnessRegistry{adapters: make(map[string]HarnessAdapter)}
}

func (r *HarnessRegistry) Register(adapter HarnessAdapter) error {
	if adapter == nil {
		return errors.New("harness adapter is nil")
	}
	descriptor := adapter.Describe()
	name := strings.TrimSpace(descriptor.Name)
	if name == "" || name != descriptor.Name {
		return fmt.Errorf("harness adapter name %q is invalid", descriptor.Name)
	}
	if !descriptor.Supports(CapabilityStreaming) {
		return fmt.Errorf("harness adapter %q must declare %s", name, CapabilityStreaming)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("harness adapter %q is already registered", name)
	}
	r.adapters[name] = adapter
	return nil
}

func (r *HarnessRegistry) Get(name string) (HarnessAdapter, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	adapter, ok := r.adapters[harnessName(name)]
	return adapter, ok
}

func (r *HarnessRegistry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AdapterFor returns the adapter that runs profile, after checking that it
// declares every capability the profile requires.
func (r *HarnessRegistry) AdapterFor(profile AgentServiceProfile) (HarnessAdapter, error) {
	name := harnessName(profile.Harness)
	adapter, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: service %q uses harness %q", ErrHarnessNotRegistered, profile.Slug, name)
	}
	if validator, ok := adapter.(HarnessProfileValidator); ok {
		if err := validator.ValidateProfile(profile); err != nil {
			return nil, fmt.Errorf("%w: service %q: %v", ErrHarnessProfileInvalid, profile.Slug, err)
		}
	}
	descriptor := adapter.Describe()
	var missing []string
	for _, capability := range RequiredHarnessCapabilities(profile) {
		if !descriptor.Supports(capability) {
			missing = append(missing, string(capability))
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: service %q needs %s from harness %q", ErrHarnessCapabilityMissing, profile.Slug, strings.Join(missing, ", "), name)
	}
	return adapter, nil
}

// CheckProfile reports whether profile can run on a registered adapter.
func (r *HarnessRegistry) CheckProfile(profile AgentServiceProfile) error {
	_, err := r.AdapterFor(profile)
	return err
}

// RequiredHarnessCapabilities derives what a service needs from its harness.
// Usage, context capacity and history summary are optional: SemOS has a
// fallback for each.
func RequiredHarnessCapabilities(profile AgentServiceProfile) []HarnessCapability {
	required := []HarnessCapability{CapabilityStreaming, CapabilityCancellation}
	if profile.PermissionDefault == PermissionAsk {
		required = append(required, CapabilityInteractiveToolApproval)
	}
	if profile.SaveAndResumeConversations {
		required = append(required, CapabilityHistoryReplay)
	}
	return required
}

func harnessName(name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return DefaultHarness
}

// CheckActiveProfiles returns one error per active service whose harness is
// unregistered or lacks a required capability. Startup logs them; the
// services stay unavailable until fixed.
func CheckActiveProfiles(ctx context.Context, profiles *ProfileRegistry, harnesses *HarnessRegistry) []error {
	var active []AgentServiceProfile
	if profiles.VersionStoreReady() {
		stored, err := profiles.versionStore.ListActive(ctx)
		if err != nil {
			return []error{fmt.Errorf("list active agent service profiles: %w", err)}
		}
		active = stored
	} else {
		for slug, version := range profiles.active {
			active = append(active, profiles.versions[slug][version])
		}
	}
	var problems []error
	for _, profile := range active {
		if err := harnesses.CheckProfile(profile); err != nil {
			problems = append(problems, err)
		}
	}
	return problems
}
