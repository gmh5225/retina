// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package tcpretrans

import (
	"context"
	"net"
	"strings"

	kcfg "github.com/microsoft/retina/pkg/config"
	"github.com/microsoft/retina/pkg/enricher"
	"github.com/microsoft/retina/pkg/log"
	"github.com/microsoft/retina/pkg/plugin/api"
	"github.com/microsoft/retina/pkg/utils"
	v1 "github.com/cilium/cilium/pkg/hubble/api/v1"
	"github.com/cilium/ebpf/rlimit"
	gadgetcontext "github.com/inspektor-gadget/inspektor-gadget/pkg/gadget-context"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/gadgets/trace/tcpretrans/tracer"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/gadgets/trace/tcpretrans/types"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

func New(cfg *kcfg.Config) api.Plugin {
	return &tcpretrans{
		cfg: cfg,
		l:   log.Logger().Named(string(Name)),
	}
}

func (t *tcpretrans) Name() string {
	return string(Name)
}

func (t *tcpretrans) Generate(ctx context.Context) error {
	return nil
}

func (t *tcpretrans) Compile(ctx context.Context) error {
	return nil
}

func (t *tcpretrans) Init() error {
	if !t.cfg.EnablePodLevel {
		t.l.Warn("tcpretrans will not init because pod level is disabled")
		return nil
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		t.l.Error("RemoveMemLock failed", zap.Error(err))
		return err
	}

	// Create tracer. In this case no parameters are passed.
	t.tracer = &tracer.Tracer{}
	t.tracer.SetEventHandler(t.eventHandler)
	t.l.Info("Initialized tcpretrans plugin")
	return nil
}

func (t *tcpretrans) Start(ctx context.Context) error {
	if !t.cfg.EnablePodLevel {
		t.l.Warn("tcpretrans will not start because pod level is disabled")
		return nil
	}
	t.enricher = enricher.Instance()
	if t.enricher == nil {
		t.l.Error("Failed to get enricher instance")
		return errors.New("failed to get enricher instance")
	}
	t.gadgetCtx = gadgetcontext.New(ctx, "tcpretrans", nil, nil, nil, nil, nil, nil, nil, nil, 0, nil)

	err := t.tracer.Run(t.gadgetCtx)
	if err != nil {
		t.l.Error("Failed to run tracer", zap.Error(err))
		return err
	}
	t.l.Info("Started tcpretrans plugin")
	return nil
}

func (t *tcpretrans) Stop() error {
	if !t.cfg.EnablePodLevel {
		return nil
	}
	if t.gadgetCtx == nil {
		t.l.Warn("tcpretrans plugin does not have a gadget context")
		return nil
	}
	t.gadgetCtx.Cancel()
	t.l.Info("Stopped tcpretrans plugin")
	return nil
}

func (t *tcpretrans) SetupChannel(ch chan *v1.Event) error {
	t.l.Warn("SetupChannel is not supported by plugin", zap.String("plugin", string(Name)))
	return nil
}

func (t *tcpretrans) eventHandler(event *types.Event) {
	if event == nil {
		return
	}

	if event.IPVersion != 4 {
		return
	}

	// TODO add metric here or add a enriched value
	fl := utils.ToFlow(
		int64(event.Timestamp),
		net.ParseIP(event.SrcEndpoint.L3Endpoint.Addr).To4(), // Precautionary To4() call.
		net.ParseIP(event.DstEndpoint.L3Endpoint.Addr).To4(), // Precautionary To4() call.
		uint32(event.SrcEndpoint.Port),
		uint32(event.DstEndpoint.Port),
		unix.IPPROTO_TCP, // only TCP can  have retransmissions
		0,                // drop reason packet doesn't have a direction yet, so we set it to 0
		utils.Verdict_RETRANSMISSION,
		0,
	)

	if fl == nil {
		t.l.Warn("Could not convert tracer Event to flow", zap.Any("tracer event", event))
		return
	}
	syn, ack, fin, rst, psh, urg := getTcpFlags(event.Tcpflags)
	utils.AddTcpFlags(fl, syn, ack, fin, rst, psh, urg)

	// This is only for development purposes.
	// Removing this makes logs way too chatter-y.
	// dr.l.Debug("DropReason Packet Received", zap.Any("flow", fl), zap.Any("Raw Bpf Event", bpfEvent), zap.Uint32("drop type", bpfEvent.Key.DropType))

	// Write the event to the enricher.
	t.enricher.Write(&v1.Event{
		Event:     fl,
		Timestamp: fl.Time,
	})
}

func getTcpFlags(flags string) (syn, ack, fin, rst, psh, urg uint16) {
	// this limiter is used in IG to put all the flags together
	syn, ack, fin, rst, psh, urg = 0, 0, 0, 0, 0, 0
	result := strings.Split(flags, "|")
	for _, flag := range result {
		switch flag {
		case "SYN":
			syn = 1
		case "ACK":
			ack = 1
		case "FIN":
			fin = 1
		case "RST":
			rst = 1
		case "PSH":
			psh = 1
		case "URG":
			urg = 1
		}
	}
	return
}