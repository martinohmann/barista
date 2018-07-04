// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package vpn provides an i3bar module for openvpn information.
package vpn

import (
	"net"

	"golang.org/x/sys/unix"

	"github.com/vishvananda/netlink"

	"github.com/soumya92/barista/bar"
	"github.com/soumya92/barista/base"
	l "github.com/soumya92/barista/logging"
)

// State represents the vpn state.
type State int

// Connected returns true if the VPN is connected.
func (s State) Connected() bool {
	return s == Connected
}

// Disconnected returns true if the VPN is off.
func (s State) Disconnected() bool {
	return s == Disconnected
}

// Valid states for the vpn
const (
	Connected State = iota
	Waiting
	Disconnected
)

// Module represents a VPN bar module.
type Module struct {
	base.SimpleClickHandler
	intf       string
	outputFunc base.Value // of func(State) bar.Output
}

// New constructs an instance of the VPN module for the specified interface.
func New(iface string) *Module {
	m := &Module{intf: iface}
	l.Label(m, iface)
	l.Register(m, "outputFunc")
	// Default output template that's just 'VPN' when connected.
	m.Template("{{if .Connected}}VPN{{end}}")
	return m
}

// DefaultInterface constructs an instance of the VPN module for "tun0",
// the usual interface for VPNs.
func DefaultInterface() *Module {
	return New("tun0")
}

// Output configures a module to display the output of a user-defined function.
func (m *Module) Output(outputFunc func(State) bar.Output) *Module {
	m.outputFunc.Set(outputFunc)
	return m
}

// Template configures a module to display the output of a template.
func (m *Module) Template(template string) *Module {
	base.Template(template, m.Output)
	return m
}

// Stream starts the module.
func (m *Module) Stream(s bar.Sink) {
	// Initial state.
	state := Disconnected
	if link, err := netlink.LinkByName(m.intf); err == nil {
		if link.Attrs().Flags&net.FlagUp == net.FlagUp {
			state = Connected
		} else {
			state = Waiting
		}
	}
	outputFunc := m.outputFunc.Get().(func(State) bar.Output)
	s.Output(outputFunc(state))

	// Watch for changes.
	linkCh := make(chan netlink.LinkUpdate)
	done := make(chan struct{})
	defer close(done)
	netlink.LinkSubscribe(linkCh, done)
	var lastFlags uint32

	for {
		select {
		case update := <-linkCh:
			if update.Attrs().Name != m.intf {
				continue
			}
			newFlags := update.IfInfomsg.Flags
			shouldUpdate := false
			if lastFlags&unix.IFF_UP != newFlags&unix.IFF_UP {
				shouldUpdate = true
			}
			if lastFlags&unix.IFF_RUNNING != newFlags&unix.IFF_RUNNING {
				shouldUpdate = true
			}
			if shouldUpdate {
				lastFlags = newFlags
				if newFlags&unix.IFF_RUNNING == unix.IFF_RUNNING {
					state = Connected
				} else if newFlags&unix.IFF_UP == unix.IFF_UP {
					state = Waiting
				} else {
					state = Disconnected
				}
				s.Output(outputFunc(state))
			}
		case <-m.outputFunc.Update():
			outputFunc = m.outputFunc.Get().(func(State) bar.Output)
			s.Output(outputFunc(state))
		}
	}
}
