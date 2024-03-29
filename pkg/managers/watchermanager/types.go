// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package watchermanager

import (
	"context"
	"sync"
	"time"

	"github.com/microsoft/retina/pkg/log"
)

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=types.go -destination=mocks/mock_types.go -package=mocks .
type IWatcher interface {
	// Init, Stop, and Refresh should only be called by watchermanager.
	Init(ctx context.Context) error
	Stop(ctx context.Context) error
	Refresh(ctx context.Context) error
}

type IWatcherManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type WatcherManager struct {
	Watchers    []IWatcher
	l           *log.ZapLogger
	refreshRate time.Duration
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}
