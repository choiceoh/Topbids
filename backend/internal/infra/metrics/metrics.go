// Package metrics provides lightweight Prometheus-compatible instrumentation.
// No external dependencies — stdlib only.
// Ported from Deneb gateway.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Counter struct {
	mu     sync.RWMutex
	values map[string]*atomic.Int64
	name   string
	help   string
	labels []string
}

func NewCounter(name, help string, labels ...string) *Counter {
	return &Counter{values: make(map[string]*atomic.Int64), name: name, help: help, labels: labels}
}

func (c *Counter) Inc(labelValues ...string) {
	key := strings.Join(labelValues, "\x00")
	c.mu.RLock()
	v, ok := c.values[key]
	c.mu.RUnlock()
	if ok {
		v.Add(1)
		return
	}
	c.mu.Lock()
	if v, ok = c.values[key]; ok {
		c.mu.Unlock()
		v.Add(1)
		return
	}
	v = &atomic.Int64{}
	v.Store(1)
	c.values[key] = v
	c.mu.Unlock()
}

func (c *Counter) writeTo(w io.Writer) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.values) == 0 {
		return
	}
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", c.name, c.help, c.name)
	keys := sortedKeys(c.values)
	for _, key := range keys {
		fmt.Fprintf(w, "%s%s %d\n", c.name, formatLabels(c.labels, key), c.values[key].Load())
	}
}

type Gauge struct {
	value atomic.Int64
	name  string
	help  string
}

func NewGauge(name, help string) *Gauge {
	return &Gauge{name: name, help: help}
}

func (g *Gauge) Inc()        { g.value.Add(1) }
func (g *Gauge) Dec()        { g.value.Add(-1) }
func (g *Gauge) Set(v int64) { g.value.Store(v) }
func (g *Gauge) Load() int64 { return g.value.Load() }

func (g *Gauge) writeTo(w io.Writer) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", g.name, g.help, g.name, g.name, g.value.Load())
}

type Histogram struct {
	mu      sync.RWMutex
	series  map[string]*histogramData
	name    string
	help    string
	labels  []string
	buckets []float64
}

type histogramData struct {
	bucketCounts []atomic.Int64
	count        atomic.Int64
	sumMicros    atomic.Int64
}

func NewHistogram(name, help string, buckets []float64, labels ...string) *Histogram {
	return &Histogram{series: make(map[string]*histogramData), name: name, help: help, labels: labels, buckets: buckets}
}

func (h *Histogram) Observe(value float64, labelValues ...string) {
	key := strings.Join(labelValues, "\x00")
	h.mu.RLock()
	d, ok := h.series[key]
	h.mu.RUnlock()
	if !ok {
		h.mu.Lock()
		if d, ok = h.series[key]; !ok {
			d = &histogramData{bucketCounts: make([]atomic.Int64, len(h.buckets))}
			h.series[key] = d
		}
		h.mu.Unlock()
	}
	d.count.Add(1)
	d.sumMicros.Add(int64(value * 1e6))
	for i, bound := range h.buckets {
		if value <= bound {
			d.bucketCounts[i].Add(1)
			break
		}
	}
}

func (h *Histogram) ObserveDuration(start time.Time, labelValues ...string) {
	h.Observe(time.Since(start).Seconds(), labelValues...)
}

func (h *Histogram) writeTo(w io.Writer) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.series) == 0 {
		return
	}
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s histogram\n", h.name, h.help, h.name)
	for _, key := range sortedMapKeys(h.series) {
		d := h.series[key]
		labelStr := formatLabels(h.labels, key)
		cumulative := int64(0)
		for i, bound := range h.buckets {
			cumulative += d.bucketCounts[i].Load()
			le := fmt.Sprintf("%g", bound)
			if labelStr == "" {
				fmt.Fprintf(w, "%s_bucket{le=\"%s\"} %d\n", h.name, le, cumulative)
			} else {
				fmt.Fprintf(w, "%s_bucket{%s,le=\"%s\"} %d\n", h.name, labelStr[1:len(labelStr)-1], le, cumulative)
			}
		}
		count := d.count.Load()
		if labelStr == "" {
			fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", h.name, count)
		} else {
			fmt.Fprintf(w, "%s_bucket{%s,le=\"+Inf\"} %d\n", h.name, labelStr[1:len(labelStr)-1], count)
		}
		fmt.Fprintf(w, "%s_sum%s %g\n", h.name, labelStr, float64(d.sumMicros.Load())/1e6)
		fmt.Fprintf(w, "%s_count%s %d\n", h.name, labelStr, count)
	}
}

// --- Phaeton metrics ---

var (
	HTTPRequestsTotal = NewCounter("phaeton_http_requests_total", "Total HTTP requests.", "method", "path", "status")
	HTTPDuration      = NewHistogram("phaeton_http_duration_seconds", "HTTP request duration.",
		[]float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}, "method")
	ActiveApps = NewGauge("phaeton_active_apps", "Number of works apps.")
)

var allMetrics = []interface{ writeTo(io.Writer) }{
	HTTPRequestsTotal, HTTPDuration, ActiveApps,
}

func WriteMetrics(w io.Writer) {
	for _, m := range allMetrics {
		m.writeTo(w)
	}
}

func formatLabels(names []string, key string) string {
	if len(names) == 0 {
		return ""
	}
	parts := strings.Split(key, "\x00")
	var b strings.Builder
	b.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		val := ""
		if i < len(parts) {
			val = parts[i]
		}
		fmt.Fprintf(&b, "%s=%q", name, val)
	}
	b.WriteByte('}')
	return b.String()
}

func sortedKeys(m map[string]*atomic.Int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeys(m map[string]*histogramData) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
