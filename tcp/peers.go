package tcp

import (
	"sync"

	msg "github.com/UniversityRadioYork/bifrost-go/message"
)

type Peers struct {
	m  map[string]chan<- msg.Message
	mu sync.RWMutex
}

var peers *Peers

func init() {
	peers = &Peers{m: make(map[string]chan<- msg.Message)}
}

// Add creates and returns a new channel for the given peer address.
// If an address already exists in the registry, it returns nil.
func (p *Peers) Add(addr string) <-chan msg.Message {
	p.mu.Lock()
	defer p.mu.Unlock()

	// If addr key already exists, return nil
	if _, ok := p.m[addr]; ok {
		return nil
	}

	newChan := make(chan msg.Message)
	p.m["addr"] = newChan
	return newChan
}

// Remove deletes the specified peer from the registry.
func (p *Peers) Remove(addr string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.m, addr)
}

// List returns a slice of all active peer channels.
func (p *Peers) List() []chan<- msg.Message {
	p.mu.RLock()
	defer p.mu.RUnlock()

	chans := make([]chan<- msg.Message, 0, len(p.m))

	for _, v := range p.m {
		chans = append(chans, v)
	}
	return chans
}
