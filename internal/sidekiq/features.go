package sidekiq

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Capabilities is an immutable selection of optional functionality.
type Capabilities struct {
	queuePause bool
}

// SupportsQueuePause reports whether queue pause support was explicitly enabled.
func (c Capabilities) SupportsQueuePause() bool { return c.queuePause }

type feature struct {
	requires     []string
	capabilities Capabilities
}

// Each feature contributes capabilities and may compose other registered features.
var featureRegistry = map[string]feature{
	"pro": {capabilities: Capabilities{queuePause: true}},
	"ent": {requires: []string{"pro"}},
}

// FeatureNames returns the available optional features in alphabetical order.
func FeatureNames() []string {
	return slices.Sorted(maps.Keys(featureRegistry))
}

// ResolveFeatures combines explicitly selected features, rejecting unknown names.
func ResolveFeatures(names []string) (Capabilities, error) {
	var result Capabilities
	enabled := make(map[string]bool)
	var enable func(string) error
	enable = func(name string) error {
		if enabled[name] {
			return nil
		}
		definition, ok := featureRegistry[name]
		if !ok {
			return fmt.Errorf("unknown feature %q (available: %s)", name, strings.Join(FeatureNames(), ", "))
		}
		enabled[name] = true
		for _, dependency := range definition.requires {
			if err := enable(dependency); err != nil {
				return err
			}
		}
		result.queuePause = result.queuePause || definition.capabilities.queuePause
		return nil
	}
	for _, name := range names {
		if err := enable(name); err != nil {
			return Capabilities{}, err
		}
	}
	return result, nil
}

// GetCapabilities returns the client's explicit configuration without Redis I/O.
func (c *Client) GetCapabilities() Capabilities { return c.capabilities }
