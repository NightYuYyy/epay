package provider

import "sync"

// ProviderFactory constructs a provider instance using its instance ID and
// decrypted string configuration.
type ProviderFactory func(instanceID string, config map[string]string) (Provider, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]ProviderFactory)
)

// Register adds or replaces a provider factory for key.
func Register(key string, factory ProviderFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[key] = factory
}

// Get returns the provider factory registered for key.
func Get(key string) (ProviderFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	factory, ok := registry[key]
	return factory, ok
}

// List returns all registered provider keys.
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	keys := make([]string, 0, len(registry))
	for key := range registry {
		keys = append(keys, key)
	}
	return keys
}
