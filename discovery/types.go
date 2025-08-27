package discovery

import (
	"sync"

	"github.com/JPKribs/FinGuard/internal"
	"github.com/godbus/dbus/v5"
	"github.com/grandcat/zeroconf"
	"github.com/holoplot/go-avahi"
)

// MARK: mdns.go

type Discovery struct {
	logger      *internal.Logger
	conn        *dbus.Conn
	server      *avahi.Server
	entryGroups map[string]*avahi.EntryGroup
	servers     map[string]*zeroconf.Server
	mu          sync.RWMutex
	running     bool
	localIP     string
	stopChan    chan struct{}
	hostName    string
	useAvahi    bool
}
