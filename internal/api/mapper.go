package api

import (
	"time"

	"github.com/sanverite/simple-packet-logger/internal/core"
)

// FromCoreSnapshot converts core.Snapshot to the public StatusResponse.
// It computes uptime based on StartedAt and current wall-clock time.
func FromCoreSnapshot(s core.Snapshot) StatusResponse {
	var started string
	var uptime int64
	if !s.StartedAt.IsZero() {
		started = s.StartedAt.UTC().Format(time.RFC3339)
		uptime = int64(time.Since(s.StartedAt).Seconds())
	}

	var lastChecked string
	if !s.LastProbe.LastChecked.IsZero() {
		lastChecked = s.LastProbe.LastChecked.UTC().Format(time.RFC3339)
	}

	// Defensive copies of slices/maps are already present in core.Snapshot,
	// but we still treat them immutably on the API side.
	return StatusResponse{
		State:     string(s.AgentState),
		StartedAt: started,
		UptimeSec: uptime,
		Warnings:  append([]string(nil), s.Warnings...),
		TUN: TUNView{
			Name:    s.TUN.Name,
			Up:      s.TUN.Up,
			MTU:     s.TUN.MTU,
			LocalIP: s.TUN.LocalIP,
			PeerIP:  s.TUN.PeerIP,
		},
		Routes: RoutesView{
			DefaultVia:      s.Routes.DefaultVia,
			LanCIDRs:        append([]string(nil), s.Routes.LanCIDRs...),
			BypassHosts:     append([]string(nil), s.Routes.BypassHosts...),
			ProxyHostRoute:  s.Routes.ProxyHostRoute,
			OriginalGateway: s.Routes.OriginalGateway,
		},
		Tun2Socks: Tun2SocksView{
			PID:       s.Tun2Socks.PID,
			UptimeSec: s.Tun2Socks.UptimeSec,
			TCPOk:     s.Tun2Socks.TCPOk,
			UDPOk:     s.Tun2Socks.UDPOk,
		},
		LastProbe: ProbeView{
			Reachable:   s.LastProbe.Reachable,
			SocksOK:     s.LastProbe.SocksOK,
			ConnectOK:   s.LastProbe.ConnectOK,
			UDPOK:       s.LastProbe.UDPOK,
			LatenciesMs: cloneLatencies(s.LastProbe.LatenciesMs),
			Features: ProxyFeatures{
				Auth: s.LastProbe.Features.Auth,
				IPv6: s.LastProbe.Features.IPv6,
				UDP:  s.LastProbe.Features.UDP,
			},
			LastChecked: lastChecked,
			Warnings:    append([]string(nil), s.LastProbe.Warnings...),
		},
		GeneratedAt: TimeNow().UTC().Format(time.RFC3339),
	}
}

// FromProbeSummary converts core.ProbeSummary to the public ProbeView.
// Keeps slice/map fields immutable by cloning.
func FromProbeSummary(p core.ProbeSummary) ProbeView {
	var lastChecked string
	if !p.LastChecked.IsZero() {
		lastChecked = p.LastChecked.UTC().Format(time.RFC3339)
	}
	return ProbeView{
		Reachable:   p.Reachable,
		SocksOK:     p.SocksOK,
		ConnectOK:   p.ConnectOK,
		UDPOK:       p.UDPOK,
		LatenciesMs: cloneLatencies(p.LatenciesMs),
		Features: ProxyFeatures{
			Auth: p.Features.Auth,
			IPv6: p.Features.IPv6,
			UDP:  p.Features.UDP,
		},
		LastChecked: lastChecked,
		Warnings:    append([]string(nil), p.Warnings...),
	}
}

func cloneLatencies(in map[string]int64) map[string]int64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
