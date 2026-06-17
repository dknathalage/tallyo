package llm

import (
	"context"
	"fmt"
	"sync"
)

// Fake is a scripted Client for tests: each CreateMessage returns the next
// queued Response. It records the requests it received for assertions.
type Fake struct {
	mu        sync.Mutex
	responses []Response
	i         int
	Requests  []Request
}

func NewFake(responses ...Response) *Fake { return &Fake{responses: responses} }

func (f *Fake) CreateMessage(_ context.Context, req Request) (*Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests = append(f.Requests, req)
	if f.i >= len(f.responses) {
		return nil, fmt.Errorf("fake llm: no scripted response for call %d", f.i+1)
	}
	r := f.responses[f.i]
	f.i++
	return &r, nil
}

// SetResponses replaces the scripted responses and resets the cursor. Useful
// when a test must build responses that depend on values known only after the
// agent (and its DB fixtures) are constructed.
func (f *Fake) SetResponses(rs ...Response) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = rs
	f.i = 0
}

func (f *Fake) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.i
}
