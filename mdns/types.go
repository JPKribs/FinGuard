package mdns

import (
	"sync"

	"github.com/JPKribs/FinGuard/internal"
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
)

// MARK: Discovery
type Discovery struct {
	logger      *internal.Logger
	server      *avahi.Server
	conn        *dbus.Conn
	entryGroups map[string]*avahi.EntryGroup
	localIP     string
	hostName    string
	running     bool
	stopChan    chan struct{}
	mu          sync.RWMutex
}
