package logging

import (
	"fmt"
	"io"
	"time"
)

type BannerInfo struct {
	Version string
	Addr    string
	DB      string // "connected" or "offline"
}

func PrintBanner(w io.Writer, info BannerInfo, color bool) {
	dim := pick(color, ansiDim)
	bold := pick(color, ansiBold)
	cyan := pick(color, ansiBoldCyn)
	green := pick(color, ansiBoldGrn)
	reset := pick(color, ansiReset)

	fmt.Fprintf(w, "\n  %s%s✦%s %stopworks%s\n", cyan, bold, reset, bold, reset)
	fmt.Fprintf(w, "  %s%s%s\n\n", dim, info.Version, reset)

	kv := func(key, val string) {
		fmt.Fprintf(w, "  %s%-10s%s%s\n", dim, key, reset, val)
	}
	kv("addr", info.Addr)
	if info.DB != "" {
		kv("db", info.DB)
	}

	fmt.Fprintf(w, "\n  %sready.%s\n\n", green, reset)
}

func PrintShutdown(w io.Writer, uptime time.Duration, color bool) {
	dim := pick(color, ansiDim)
	bold := pick(color, ansiBold)
	reset := pick(color, ansiReset)
	fmt.Fprintf(w, "\n  %stopworks stopped%s  %s(%s)%s\n\n",
		bold, reset, dim, formatUptime(uptime), reset)
}

func formatUptime(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func pick(color bool, a string) string {
	if color {
		return a
	}
	return ""
}
