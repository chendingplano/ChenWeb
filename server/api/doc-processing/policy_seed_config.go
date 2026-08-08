// policy_seed_config.go
package docprocessing

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// DocProcessingPolicySeedPolicy is one [doc-processing-policy-<name>]
// section: a named, human-described processor allow-list, sourced from
// config.local.toml (see docs/superpowers/specs/2026-08-08-doc-processing-policy-design.md).
type DocProcessingPolicySeedPolicy struct {
	Description string
	IsDefault   bool
	Processors  []string
}

// DocProcessingPolicySeedConfig is every [doc-processing-policy-*] section
// in a config file, keyed by the section-name suffix after
// "doc-processing-policy-" (e.g. "no-entities-relations", "all"), plus the
// [doc-processing-policy-bindings] knowledge-store-name -> policy-name map.
type DocProcessingPolicySeedConfig struct {
	Policies map[string]DocProcessingPolicySeedPolicy
	Bindings map[string]string
}

const (
	docProcessingPolicySeedSectionPrefix = "doc-processing-policy-"
	docProcessingPolicySeedBindingsKey   = docProcessingPolicySeedSectionPrefix + "bindings"
)

// LoadDocProcessingPolicySeedConfig reads and parses path.
func LoadDocProcessingPolicySeedConfig(path string) (DocProcessingPolicySeedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DocProcessingPolicySeedConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseDocProcessingPolicySeedConfig(data)
}

// ParseDocProcessingPolicySeedConfig parses raw TOML bytes. Sections not
// prefixed with "doc-processing-policy-" are ignored, so the same file can
// hold unrelated config (as config.local.toml does).
func ParseDocProcessingPolicySeedConfig(data []byte) (DocProcessingPolicySeedConfig, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return DocProcessingPolicySeedConfig{}, fmt.Errorf("parse toml: %w", err)
	}
	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{},
		Bindings: map[string]string{},
	}
	for key, value := range raw {
		if !strings.HasPrefix(key, docProcessingPolicySeedSectionPrefix) {
			continue
		}
		section, ok := value.(map[string]any)
		if !ok {
			return DocProcessingPolicySeedConfig{}, fmt.Errorf("%s: expected a table", key)
		}
		if key == docProcessingPolicySeedBindingsKey {
			for store, policyVal := range section {
				policyName, ok := policyVal.(string)
				if !ok {
					return DocProcessingPolicySeedConfig{}, fmt.Errorf("%s.%s: expected a string policy name", key, store)
				}
				cfg.Bindings[store] = strings.TrimPrefix(policyName, docProcessingPolicySeedSectionPrefix)
			}
			continue
		}
		name := strings.TrimPrefix(key, docProcessingPolicySeedSectionPrefix)
		policy, err := decodeDocProcessingPolicySeedPolicy(section)
		if err != nil {
			return DocProcessingPolicySeedConfig{}, fmt.Errorf("%s: %w", key, err)
		}
		cfg.Policies[name] = policy
	}
	return cfg, nil
}

func decodeDocProcessingPolicySeedPolicy(section map[string]any) (DocProcessingPolicySeedPolicy, error) {
	var policy DocProcessingPolicySeedPolicy
	if v, ok := section["description"]; ok {
		s, ok := v.(string)
		if !ok {
			return DocProcessingPolicySeedPolicy{}, fmt.Errorf("description must be a string")
		}
		policy.Description = s
	}
	if v, ok := section["is_default"]; ok {
		b, ok := v.(bool)
		if !ok {
			return DocProcessingPolicySeedPolicy{}, fmt.Errorf("is_default must be a boolean")
		}
		policy.IsDefault = b
	}
	if v, ok := section["processors"]; ok {
		list, ok := v.([]any)
		if !ok {
			return DocProcessingPolicySeedPolicy{}, fmt.Errorf("processors must be an array")
		}
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return DocProcessingPolicySeedPolicy{}, fmt.Errorf("processors entries must be strings")
			}
			policy.Processors = append(policy.Processors, s)
		}
	}
	return policy, nil
}

// Validate enforces the doc-processing-policy-seed rules: at least one
// policy, exactly one is_default = true, every processor name resolves
// against the real production processor registry, and every binding names
// a policy that exists in this same config.
func (c DocProcessingPolicySeedConfig) Validate() error {
	if len(c.Policies) == 0 {
		return fmt.Errorf("no [doc-processing-policy-*] sections found")
	}
	var defaultName string
	for name, policy := range c.Policies {
		if len(policy.Processors) == 0 {
			return fmt.Errorf("policy %q: processors must not be empty", name)
		}
		if err := validateRequiredProcessors(policy.Processors); err != nil {
			return fmt.Errorf("policy %q: %w", name, err)
		}
		if policy.IsDefault {
			if defaultName != "" {
				return fmt.Errorf("multiple default policies: %q and %q", defaultName, name)
			}
			defaultName = name
		}
	}
	if defaultName == "" {
		return fmt.Errorf("no policy has is_default = true")
	}
	for store, policyName := range c.Bindings {
		if _, ok := c.Policies[policyName]; !ok {
			return fmt.Errorf("binding %q: unknown policy %q", store, policyName)
		}
	}
	return nil
}

// DefaultPolicyName returns the name of the policy with IsDefault = true.
// Callers must call Validate first; DefaultPolicyName returns "" if no
// policy is marked default (Validate would have already rejected that).
func (c DocProcessingPolicySeedConfig) DefaultPolicyName() string {
	for name, policy := range c.Policies {
		if policy.IsDefault {
			return name
		}
	}
	return ""
}
