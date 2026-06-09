package balance

import (
	"context"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// Manager holds balance providers and periodically refreshes them.
type Manager struct {
	factory     *Factory
	providers   map[string]Provider // channel_name -> provider
	results     map[string]*Info    // channel_name -> last fetched info
	mu          sync.RWMutex
	lifecycleMu sync.Mutex
	wg          sync.WaitGroup
	interval    time.Duration
	stopCh      chan struct{}
	running     bool
}

// NewManager creates a balance manager with the given sync interval.
func NewManager(interval time.Duration) *Manager {
	return &Manager{
		factory:   NewFactory(),
		providers: make(map[string]Provider),
		results:   make(map[string]*Info),
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// Register adds a channel to be monitored.
func (m *Manager) Register(channelName, providerType, baseURL, apiKey, extraConfig string) error {
	p, err := m.factory.Create(providerType, channelName, baseURL, apiKey, extraConfig)
	if err != nil {
		return err
	}
	if p == nil {
		return nil // unsupported provider type, skip silently
	}
	m.mu.Lock()
	m.providers[channelName] = p
	m.mu.Unlock()
	return nil
}

// Unregister removes a channel from monitoring.
func (m *Manager) Unregister(channelName string) {
	m.mu.Lock()
	delete(m.providers, channelName)
	delete(m.results, channelName)
	m.mu.Unlock()
}

// Get returns the last fetched balance info for a channel.
func (m *Manager) Get(channelName string) *Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.results[channelName]
}

// GetAll returns all last fetched balance infos.
func (m *Manager) GetAll() map[string]*Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copy := make(map[string]*Info, len(m.results))
	for k, v := range m.results {
		copy[k] = v
	}
	return copy
}

// RefreshAll immediately fetches balance for all registered providers.
func (m *Manager) RefreshAll() {
	m.mu.RLock()
	providers := make(map[string]Provider, len(m.providers))
	for k, v := range m.providers {
		providers[k] = v
	}
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for name, p := range providers {
		wg.Add(1)
		go func(n string, provider Provider) {
			defer wg.Done()
			info, err := provider.Fetch(ctx)
			if err != nil {
				middleware.GetLogger().Warn("balance fetch failed",
					zap.String("channel", n), zap.Error(err))
				m.mu.Lock()
				m.results[n] = &Info{
					ChannelName: n,
					Error:       err.Error(),
					FetchedAt:   time.Now(),
				}
				m.mu.Unlock()
				return
			}
			m.mu.Lock()
			m.results[n] = info
			m.mu.Unlock()
		}(name, p)
	}
	wg.Wait()
}

// Start begins the background refresh loop.
func (m *Manager) Start() {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.running {
		return
	}
	stopCh := m.stopCh
	m.running = true
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		// Initial refresh
		m.RefreshAll()

		for {
			select {
			case <-ticker.C:
				m.RefreshAll()
			case <-stopCh:
				return
			}
		}
	}()
}

// Stop halts the background refresh loop.
func (m *Manager) Stop() {
	m.lifecycleMu.Lock()
	if !m.running {
		m.lifecycleMu.Unlock()
		return
	}
	close(m.stopCh)
	m.stopCh = make(chan struct{})
	m.running = false
	m.lifecycleMu.Unlock()
	m.wg.Wait()
}
