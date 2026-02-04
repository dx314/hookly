package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// Pretty logger colors and symbols
const (
	colorRed = "\033[31m"
)

const (
	symbolSuccess = "✓"
	symbolError   = "✗"
	symbolArrow   = "→"
	symbolInfo    = "•"
	symbolWarn    = "⚠"
)

// prettyHandler is a custom slog.Handler for human-friendly CLI output.
type prettyHandler struct {
	level    slog.Leveler
	out      io.Writer
	mu       sync.Mutex
	useColor bool
	attrs    []slog.Attr
	groups   []string
}

// newPrettyHandler creates a new pretty handler.
func newPrettyHandler(out io.Writer, level slog.Leveler) *prettyHandler {
	// Detect if stdout is a TTY for color support
	useColor := false
	if f, ok := out.(*os.File); ok {
		useColor = term.IsTerminal(int(f.Fd()))
	}

	return &prettyHandler{
		level:    level,
		out:      out,
		useColor: useColor,
	}
}

func (h *prettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *prettyHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Format timestamp as HH:MM:SS
	timeStr := r.Time.Format("15:04:05")

	// Get symbol and color based on level
	symbol, levelColor := h.getSymbolAndColor(r.Level)

	// Build the message line
	var sb strings.Builder

	if h.useColor {
		sb.WriteString(colorDim)
		sb.WriteString(timeStr)
		sb.WriteString(colorReset)
		sb.WriteString(" ")
		sb.WriteString(levelColor)
		sb.WriteString(symbol)
		sb.WriteString(colorReset)
		sb.WriteString(" ")
		sb.WriteString(r.Message)
	} else {
		sb.WriteString(timeStr)
		sb.WriteString(" ")
		sb.WriteString(symbol)
		sb.WriteString(" ")
		sb.WriteString(r.Message)
	}
	sb.WriteString("\n")

	// Collect attributes from both handler attrs and record attrs
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	allAttrs = append(allAttrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// Format attributes as indented key-value pairs
	if len(allAttrs) > 0 {
		// Calculate padding (9 chars for "HH:MM:SS " + 2 for "✓ ")
		padding := "         "

		for _, attr := range allAttrs {
			key := attr.Key
			val := attr.Value.String()

			// Apply group prefixes
			for _, g := range h.groups {
				key = g + "." + key
			}

			if h.useColor {
				sb.WriteString(padding)
				sb.WriteString(colorDim)
				sb.WriteString(key)
				sb.WriteString(": ")
				sb.WriteString(colorReset)
				sb.WriteString(val)
			} else {
				sb.WriteString(padding)
				sb.WriteString(key)
				sb.WriteString(": ")
				sb.WriteString(val)
			}
			sb.WriteString("\n")
		}
	}

	_, err := fmt.Fprint(h.out, sb.String())
	return err
}

func (h *prettyHandler) getSymbolAndColor(level slog.Level) (string, string) {
	switch {
	case level >= slog.LevelError:
		return symbolError, colorRed
	case level >= slog.LevelWarn:
		return symbolWarn, colorYellow
	case level >= slog.LevelInfo:
		return symbolInfo, colorGreen
	default: // Debug
		return symbolArrow, colorDim
	}
}

func (h *prettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)

	return &prettyHandler{
		level:    h.level,
		out:      h.out,
		useColor: h.useColor,
		attrs:    newAttrs,
		groups:   h.groups,
	}
}

func (h *prettyHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups = append(newGroups, name)

	return &prettyHandler{
		level:    h.level,
		out:      h.out,
		useColor: h.useColor,
		attrs:    h.attrs,
		groups:   newGroups,
	}
}

// setupLogger configures the global logger based on debug mode.
func setupLogger(debug bool) {
	if debug {
		// Debug mode: JSON output with full details
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelDebug,
			AddSource: true,
		})))
	} else {
		// Normal mode: pretty human-readable output
		slog.SetDefault(slog.New(newPrettyHandler(os.Stdout, slog.LevelInfo)))
	}
}

// formatDuration formats a duration for display.
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
