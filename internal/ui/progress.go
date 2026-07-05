package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Progress renders transient status while work runs. Implementations must be
// safe to call even when no terminal is attached (they become no-ops).
type Progress interface {
	Start(label string)
	Update(label string)
	Stop()
}

// NewProgress returns an animated spinner writing to w when enabled, else a
// no-op that produces no output (for --quiet / --json / non-TTY / CI).
func NewProgress(w io.Writer, enabled bool) Progress {
	if !enabled {
		return noopProgress{}
	}
	return &spinner{w: w}
}

type noopProgress struct{}

func (noopProgress) Start(string)  {}
func (noopProgress) Update(string) {}
func (noopProgress) Stop()         {}

type spinner struct {
	w      io.Writer
	mu     sync.Mutex
	label  string
	stopCh chan struct{}
	doneCh chan struct{}
}

func (s *spinner) Start(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	go func() {
		frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		i := 0
		for {
			select {
			case <-s.stopCh:
				close(s.doneCh)
				return
			case <-t.C:
				s.mu.Lock()
				lbl := s.label
				s.mu.Unlock()
				fmt.Fprintf(s.w, "\r%s %s ", string(frames[i%len(frames)]), lbl)
				i++
			}
		}
	}()
}

func (s *spinner) Update(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

func (s *spinner) Stop() {
	if s.stopCh == nil {
		return
	}
	close(s.stopCh)
	<-s.doneCh
	fmt.Fprint(s.w, "\r\033[K") // carriage return + clear to end of line
	s.stopCh = nil
}
