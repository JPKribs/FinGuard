package discovery

import (
	"sync"

	"github.com/JPKribs/FinGuard/internal"
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
)

// MARK: Discovery
type Discovery struct {
	logger      *internal.Logger
	conn        *dbus.Conn
	server      *avahi.Server
	entryGroups map[string]*avahi.EntryGroup
	mu          sync.RWMutex
	running     bool
	localIP     string
	stopChan    chan struct{}
	hostName    string
}
