// Copyright (c) 2021 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

//go:generate mockgen -package $GOPACKAGE -source $GOFILE -destination peer_chooser_mock.go -self_package github.com/uber/cadence/common/rpc

package rpc

import (
	"time"

	"go.uber.org/yarpc/api/peer"
	"go.uber.org/yarpc/peer/roundrobin"

	"github.com/uber/cadence/common/dynamicconfig"
	"github.com/uber/cadence/common/log"
	"github.com/uber/cadence/common/membership"
)

const defaultDNSRefreshInterval = time.Second * 10

type (
	PeerChooserOptions struct {
		// Address is the target dns address. Used by dns peer chooser.
		Address string

		// ServiceName is the name of service. Used by direct peer chooser.
		ServiceName string

		// EnableConnectionRetainingDirectChooser is used by direct peer chooser.
		// If false, yarpc's own default direct peer chooser will be used which doesn't retain connections.
		// If true, cadence's own direct peer chooser will be used which retains connections.
		EnableConnectionRetainingDirectChooser dynamicconfig.BoolPropertyFn
	}
	PeerChooserFactory interface {
		CreatePeerChooser(transport peer.Transport, opts PeerChooserOptions) (PeerChooser, error)
	}

	PeerChooser interface {
		peer.Chooser

		// UpdatePeers updates the list of peers if needed.
		UpdatePeers([]membership.HostInfo)
	}

	dnsPeerChooserFactory struct {
		interval time.Duration
		logger   log.Logger
	}

	directPeerChooserFactory struct {
		serviceName string
		logger      log.Logger
		choosers    []*directPeerChooser
	}
)

type defaultPeerChooser struct {
	peer.Chooser
}

// UpdatePeers is a no-op for defaultPeerChooser. It is added to satisfy the PeerChooser interface.
func (d *defaultPeerChooser) UpdatePeers(peers []membership.HostInfo) {}

func NewDNSPeerChooserFactory(interval time.Duration, logger log.Logger) PeerChooserFactory {
	if interval <= 0 {
		interval = defaultDNSRefreshInterval
	}

	return &dnsPeerChooserFactory{interval, logger}
}

func (f *dnsPeerChooserFactory) CreatePeerChooser(transport peer.Transport, opts PeerChooserOptions) (PeerChooser, error) {
	peerList := roundrobin.New(transport)
	peerListUpdater, err := newDNSUpdater(peerList, opts.Address, f.interval, f.logger)
	if err != nil {
		return nil, err
	}
	peerListUpdater.Start()
	return &defaultPeerChooser{Chooser: peerList}, nil
}

func NewDirectPeerChooserFactory(serviceName string, logger log.Logger) PeerChooserFactory {
	return &directPeerChooserFactory{
		logger: logger,
	}
}

func (f *directPeerChooserFactory) CreatePeerChooser(transport peer.Transport, opts PeerChooserOptions) (PeerChooser, error) {
	c := newDirectChooser(f.serviceName, transport, f.logger, opts.EnableConnectionRetainingDirectChooser)
	f.choosers = append(f.choosers, c)
	return c, nil
}
