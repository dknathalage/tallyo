package agent

import (
	"strings"

	"github.com/dknathalage/tallyo/internal/agent/llm"
)

// requestMaxTokens is the per-call output ceiling for a Smart's single model
// turn. Relocated from the deleted context.go; also used by extract.go.
const requestMaxTokens = 64000

// Smarts is the AI capability surface: a small curated set of one-shot actions,
// each gather → propose → apply. It depends only on the slice services it needs
// and the model client; no conversation/step/checkpoint state.
type Smarts struct {
	cfg     Config
	client  llm.Client
	shifts  ShiftWorker
	catalog CatalogueSearcher
}

// NewSmarts constructs the Smarts service. A nil dependency is a programmer error.
func NewSmarts(cfg Config, client llm.Client, shifts ShiftWorker, catalog CatalogueSearcher) *Smarts {
	if client == nil || shifts == nil || catalog == nil {
		panic("agent: NewSmarts requires non-nil client, shifts, catalog")
	}
	return &Smarts{cfg: cfg, client: client, shifts: shifts, catalog: catalog}
}

// wrapUntrusted fences arbitrary record text so the model treats it as data
// rather than instructions. Relocated from the deleted prompt.go; used by
// extract.go and the draft-invoice gather.
func wrapUntrusted(label, body string) string {
	sanitised := strings.ReplaceAll(body, "</untrusted-content", "&lt;/untrusted-content")
	return "<untrusted-content source=\"" + label + "\">\n" + sanitised + "\n</untrusted-content>"
}
