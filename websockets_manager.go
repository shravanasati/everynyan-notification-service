package main

import (
	"iter"
	"maps"
	"net"
	"sync"
)

type WebsocketConnectionsManager struct {
	authorConnMap map[string]net.Conn
	mu            sync.Mutex
}

func NewWebsocketConnectionsManager() *WebsocketConnectionsManager {
	return &WebsocketConnectionsManager{
		authorConnMap: make(map[string]net.Conn),
	}
}

func (manager *WebsocketConnectionsManager) Add(user string, conn net.Conn) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.authorConnMap[user] = conn
}

func (manager *WebsocketConnectionsManager) Get(user string) (net.Conn, bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	conn, ok := manager.authorConnMap[user]
	return conn, ok
}

func (manager *WebsocketConnectionsManager) Delete(user string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	delete(manager.authorConnMap, user)
}

func (manager *WebsocketConnectionsManager) All() iter.Seq[net.Conn] {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	return (maps.Values(manager.authorConnMap))
}

func (manager *WebsocketConnectionsManager) Count() int {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	return len(manager.authorConnMap)
}

func (manager *WebsocketConnectionsManager) Close() {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	for conn := range maps.Values(manager.authorConnMap) {
		conn.Close()
	}
}
