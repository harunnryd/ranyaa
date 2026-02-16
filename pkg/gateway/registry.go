package gateway

import (
	"sync"
	"time"
)

// ClientRegistry manages connected clients
type ClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// NewClientRegistry creates a new client registry
func NewClientRegistry() *ClientRegistry {
	return &ClientRegistry{
		clients: make(map[string]*Client),
	}
}

// Add adds a client to the registry
func (r *ClientRegistry) Add(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clients[client.ID] = client
}

// Remove removes a client from the registry
func (r *ClientRegistry) Remove(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.clients, clientID)
}

// Get retrieves a client by ID
func (r *ClientRegistry) Get(clientID string) (*Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, exists := r.clients[clientID]
	return client, exists
}

// GetAll returns all clients
func (r *ClientRegistry) GetAll() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

// GetAuthenticatedClients returns only authenticated clients
func (r *ClientRegistry) GetAuthenticatedClients() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make([]*Client, 0)
	for _, client := range r.clients {
		if client.Authenticated {
			clients = append(clients, client)
		}
	}
	return clients
}

// Count returns the number of connected clients
func (r *ClientRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.clients)
}

// GetConnectedClients returns client information for all connected clients
func (r *ClientRegistry) GetConnectedClients() []ClientInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	infos := make([]ClientInfo, 0, len(r.clients))

	for _, client := range r.clients {
		idle := now.Sub(client.LastActivity) > 5*time.Minute

		infos = append(infos, ClientInfo{
			ID:            client.ID,
			Authenticated: client.Authenticated,
			ConnectedAt:   client.ConnectedAt,
			LastActivity:  client.LastActivity,
			IPAddress:     client.IPAddress,
			Idle:          idle,
		})
	}

	return infos
}

// UpdateActivity updates the last activity time for a client
func (r *ClientRegistry) UpdateActivity(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if client, exists := r.clients[clientID]; exists {
		client.LastActivity = time.Now()
	}
}
