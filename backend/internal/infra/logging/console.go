// Package logging provides a human-readable console log handler for slog.
//
// Adapted from Deneb gateway (github.com/choiceoh/deneb).
//
//	14:05:09 │ [engine] app created slug=permit-checklist
package logging

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"
	"unicode"
)

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiItalic  = "\033[3m"
	ansiRed     = "\033[31m"
	ansiYellow  = "\033[33m"
	ansiCyan    = "\033[36m"
	ansiBoldRed = "\033[1;31m"
	ansiBoldGrn = "\033[1;32m"
	ansiBoldYel = "\033[1;33m"
	ansiBoldCyn = "\033[1;36m"
	ansiDimCyn  = "\033[2;36m"
)

const pkgAttrKey = "pkg"

type ConsoleHandler struct {
	w        io.Writer
	level    slog.Leveler
	color    bool
	mu       *sync.Mutex
	preAttrs []slog.Attr
	groups   []string
}

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256)
		return &b
	},
}

func NewConsoleHandler(w io.Writer, level slog.Leveler, color bool) *ConsoleHandler {
	if level == nil {
		level = slog.LevelInfo
	}
	return &ConsoleHandler{w: w, level: level, color: color, mu: &sync.Mutex{}}
}

func (h *ConsoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *ConsoleHandler) Handle(_ context.Context, r slog.Record) error {
	bp := bufPool.Get().(*[]byte)
	buf := (*bp)[:0]

	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}

	barStyle := levelBarStyle(r.Level)
	isErr := r.Level >= slog.LevelError

	pkgVal := h.pkgValue()
	if pkgVal == "" {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == pkgAttrKey {
				pkgVal = a.Value.String()
				return false
			}
			return true
		})
	}

	if h.color {
		buf = append(buf, ansiDim...)
		buf = appendTimestamp(buf, t)
		buf = append(buf, ansiReset...)
		buf = append(buf, ' ')
		buf = append(buf, barStyle...)
		buf = append(buf, "│"...)
		buf = append(buf, ansiReset...)

		if pkgVal != "" {
			buf = append(buf, ' ')
			buf = append(buf, ansiDimCyn...)
			buf = append(buf, '[')
			buf = append(buf, pkgVal...)
			buf = append(buf, ']')
			buf = append(buf, ansiReset...)
		}

		buf = append(buf, ' ')
		if isErr {
			buf = append(buf, ansiBoldRed...)
		} else {
			buf = append(buf, ansiBold...)
		}
		buf = append(buf, r.Message...)
		buf = append(buf, ansiReset...)
	} else {
		buf = appendTimestamp(buf, t)
		buf = append(buf, ' ')
		buf = append(buf, levelText(r.Level)...)
		buf = append(buf, " │ "...)
		if pkgVal != "" {
			buf = append(buf, '[')
			buf = append(buf, pkgVal...)
			buf = append(buf, "] "...)
		}
		buf = append(buf, r.Message...)
	}

	for _, a := range h.preAttrs {
		if a.Key == pkgAttrKey {
			continue
		}
		buf = h.appendAttr(buf, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == pkgAttrKey {
			return true
		}
		buf = h.appendAttr(buf, a)
		return true
	})

	buf = append(buf, '\n')
	h.mu.Lock()
	_, err := h.w.Write(buf)
	h.mu.Unlock()
	*bp = buf
	bufPool.Put(bp)
	return err
}

func (h *ConsoleHandler) pkgValue() string {
	for _, a := range h.preAttrs {
		if a.Key == pkgAttrKey {
			return a.Value.String()
		}
	}
	return ""
}

func (h *ConsoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := h.clone()
	h2.preAttrs = append(h2.preAttrs, attrs...)
	return h2
}

func (h *ConsoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

func (h *ConsoleHandler) clone() *ConsoleHandler {
	h2 := &ConsoleHandler{w: h.w, level: h.level, color: h.color, mu: h.mu}
	h2.preAttrs = make([]slog.Attr, len(h.preAttrs))
	copy(h2.preAttrs, h.preAttrs)
	h2.groups = make([]string, len(h.groups))
	copy(h2.groups, h.groups)
	return h2
}

func (h *ConsoleHandler) appendAttr(buf []byte, a slog.Attr) []byte {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return buf
	}
	isErrorKey := a.Key == "error" || a.Key == "err"
	buf = append(buf, ' ')
	if h.color {
		if isErrorKey {
			buf = append(buf, ansiItalic...)
			buf = append(buf, ansiRed...)
			buf = h.appendKey(buf, a.Key)
			buf = append(buf, ansiReset...)
			buf = append(buf, ansiRed...)
			buf = appendValue(buf, a.Value)
			buf = append(buf, ansiReset...)
		} else {
			buf = append(buf, ansiDim...)
			buf = h.appendKey(buf, a.Key)
			buf = append(buf, ansiReset...)
			buf = appendValue(buf, a.Value)
		}
	} else {
		buf = h.appendKey(buf, a.Key)
		buf = appendValue(buf, a.Value)
	}
	return buf
}

func (h *ConsoleHandler) appendKey(buf []byte, key string) []byte {
	for _, g := range h.groups {
		buf = append(buf, g...)
		buf = append(buf, '.')
	}
	buf = append(buf, key...)
	buf = append(buf, '=')
	return buf
}

func appendValue(buf []byte, v slog.Value) []byte {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if needsQuote(s) {
			buf = append(buf, strconv.Quote(s)...)
		} else {
			buf = append(buf, s...)
		}
	case slog.KindTime:
		buf = append(buf, v.Time().Format(time.RFC3339)...)
	case slog.KindDuration:
		buf = appendDuration(buf, v.Duration())
	default:
		buf = append(buf, v.String()...)
	}
	return buf
}

func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r == '"' || r == '=' || r == '\\' {
			return true
		}
	}
	return false
}

func appendTimestamp(buf []byte, t time.Time) []byte {
	h, m, s := t.Clock()
	buf = append(buf, byte('0'+h/10), byte('0'+h%10), ':')
	buf = append(buf, byte('0'+m/10), byte('0'+m%10), ':')
	buf = append(buf, byte('0'+s/10), byte('0'+s%10))
	return buf
}

func levelText(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "DBG"
	case l < slog.LevelWarn:
		return "INF"
	case l < slog.LevelError:
		return "WRN"
	default:
		return "ERR"
	}
}

func levelBarStyle(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return ansiBoldCyn
	case l < slog.LevelWarn:
		return ansiDim
	case l < slog.LevelError:
		return ansiBoldYel
	default:
		return ansiBoldRed
	}
}

func appendDuration(buf []byte, d time.Duration) []byte {
	switch {
	case d < time.Millisecond:
		buf = strconv.AppendInt(buf, d.Microseconds(), 10)
		buf = append(buf, "µs"...)
	case d < time.Second:
		buf = strconv.AppendInt(buf, d.Milliseconds(), 10)
		buf = append(buf, "ms"...)
	case d < 10*time.Second:
		tenths := d.Milliseconds() / 100
		buf = strconv.AppendFloat(buf, float64(tenths)/10.0, 'f', 1, 64)
		buf = append(buf, 's')
	default:
		buf = append(buf, d.Round(time.Second).String()...)
	}
	return buf
}
