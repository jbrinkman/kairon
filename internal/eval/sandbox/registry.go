package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ContainerEntry tracks a container in the registry
type ContainerEntry struct {
	ContainerID string    `json:"container_id"`
	Name        string    `json:"name"`
	Timestamp   time.Time `json:"timestamp"`
	EvalRun     string    `json:"eval_run"`
	Status      string    `json:"status"`
	Platform    string    `json:"platform,omitempty"`
	Image       string    `json:"image,omitempty"`
}

// Registry manages container tracking
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*ContainerEntry
	path    string
}

// NewRegistry creates a new container registry
func NewRegistry() (*Registry, error) {
	registryPath := filepath.Join(".kairon", "evals", "tmp", "containers.json")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		return nil, fmt.Errorf("creating registry directory: %w", err)
	}

	r := &Registry{
		entries: make(map[string]*ContainerEntry),
		path:    registryPath,
	}

	// Load existing entries
	if err := r.load(); err != nil && !os.IsNotExist(err) {
		// If file is corrupted, start fresh
		r.entries = make(map[string]*ContainerEntry)
	}

	return r, nil
}

// Add adds a container to the registry
func (r *Registry) Add(containerID, name, evalRun, platform, image string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := &ContainerEntry{
		ContainerID: containerID,
		Name:        name,
		Timestamp:   time.Now(),
		EvalRun:     evalRun,
		Status:      "running",
		Platform:    platform,
		Image:       image,
	}

	r.entries[containerID] = entry
	return r.save()
}

// Remove removes a container from the registry
func (r *Registry) Remove(containerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.entries, containerID)
	return r.save()
}

// List returns all containers in the registry
func (r *Registry) List() []*ContainerEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]*ContainerEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		entries = append(entries, entry)
	}
	return entries
}

// Clear removes all containers from the registry
func (r *Registry) Clear() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entries = make(map[string]*ContainerEntry)
	return r.save()
}

// UpdateStatus updates the status of a container
func (r *Registry) UpdateStatus(containerID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, exists := r.entries[containerID]; exists {
		entry.Status = status
		return r.save()
	}
	return fmt.Errorf("container %s not found in registry", containerID)
}

// Get retrieves a container entry by ID
func (r *Registry) Get(containerID string) (*ContainerEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[containerID]
	return entry, exists
}

// load reads the registry from disk
func (r *Registry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &r.entries)
}

// save writes the registry to disk
func (r *Registry) save() error {
	data, err := json.MarshalIndent(r.entries, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.path, data, 0644)
}
