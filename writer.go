package traefik_plugin_geoblock

import (
	"os"
	"sync"
	"time"
)

type bufferedFileWriter struct {
	mu        sync.Mutex
	file      *os.File
	buffer    []byte
	path      string
	maxSize   int
	timeout   time.Duration
	lastFlush time.Time
}

func newBufferedFileWriter(path string, maxSize int, timeout time.Duration) (*bufferedFileWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	w := &bufferedFileWriter{
		file:      file,
		path:      path,
		buffer:    make([]byte, 0, maxSize),
		maxSize:   maxSize,
		timeout:   timeout,
		lastFlush: time.Now(),
	}

	// Start background flush timer
	go w.flushTimer()

	return w, nil
}

func (w *bufferedFileWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buffer = append(w.buffer, p...)

	if len(w.buffer) >= w.maxSize {
		if err := w.flushLocked(); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (w *bufferedFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.flushLocked(); err != nil {
		return err
	}
	return w.file.Close()
}

func (w *bufferedFileWriter) flushTimer() {
	ticker := time.NewTicker(w.timeout)
	defer ticker.Stop()

	for range ticker.C {
		w.mu.Lock()
		if time.Since(w.lastFlush) >= w.timeout && len(w.buffer) > 0 {
			_ = w.flushLocked() // Ignore error as this is a background routine
		}
		w.mu.Unlock()
	}
}

func (w *bufferedFileWriter) flushLocked() error {
	if len(w.buffer) == 0 {
		return nil
	}

	_, err := w.file.Write(w.buffer)
	if err != nil {
		return err
	}

	w.buffer = w.buffer[:0] // Clear buffer but keep capacity
	w.lastFlush = time.Now()
	return nil
}
