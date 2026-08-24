package log

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type Level string

const (
	Debug Level = "debug"
	Info  Level = "info"
	Warn  Level = "warn"
	Error Level = "error"
)

type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	min    Level
	fields map[string]any
}

func New(out io.Writer, min Level) *Logger {
	if out == nil {
		out = os.Stderr
	}
	return &Logger{out: out, min: min, fields: map[string]any{}}
}
func (l *Logger) With(k string, v any) *Logger {
	f := map[string]any{}
	for key, val := range l.fields {
		f[key] = val
	}
	f[k] = v
	return &Logger{out: l.out, min: l.min, fields: f}
}
func rank(x Level) int {
	switch x {
	case Debug:
		return 0
	case Info:
		return 1
	case Warn:
		return 2
	default:
		return 3
	}
}
func (l *Logger) write(level Level, msg string, fields map[string]any) {
	if rank(level) < rank(l.min) {
		return
	}
	r := map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "level": level, "message": msg}
	for k, v := range l.fields {
		r[k] = v
	}
	for k, v := range fields {
		r[k] = v
	}
	data, _ := json.Marshal(r)
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(data, '\n'))
}
func (l *Logger) Debug(m string, f map[string]any) { l.write(Debug, m, f) }
func (l *Logger) Info(m string, f map[string]any)  { l.write(Info, m, f) }
func (l *Logger) Warn(m string, f map[string]any)  { l.write(Warn, m, f) }
func (l *Logger) Error(m string, f map[string]any) { l.write(Error, m, f) }
