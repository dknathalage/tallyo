package smarts

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dknathalage/tallyo/internal/reqctx"
)

const followupSystem = `You write a short, polite payment-reminder email for an overdue invoice.

Be courteous and factual. Reference the invoice number, the amount due, and the
due date. Keep it brief and professional. Do not invent details beyond what you
are given. When done, call draft_followup with the subject and body.`

// FollowUp is the editable draft returned by the follow-up Smart.
type FollowUp struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// DraftFollowUp drafts a reminder email for an overdue invoice. It gathers the
// invoice + client, makes one forced-tool call, and returns an editable draft.
// No catalogue grounding, no write.
func (s *Service) DraftFollowUp(ctx context.Context, invoiceUUID string) (FollowUp, error) {
	_ = reqctx.MustTenant(ctx) // entry-point tenant guard
	if invoiceUUID == "" {
		return FollowUp{}, fmt.Errorf("%w: invoice id required", ErrNotFound)
	}

	inv, err := s.invRead.GetByUUID(ctx, invoiceUUID)
	if err != nil {
		return FollowUp{}, err
	}
	if inv == nil {
		return FollowUp{}, ErrNotFound
	}

	user := fmt.Sprintf(
		"Overdue invoice:\n- Number: %s\n- Client: %s\n- Amount due: %.2f\n- Due date: %s\n\nWrite the reminder email.",
		inv.Number, inv.ClientName, inv.Total, inv.DueDate,
	)
	out, err := s.llm.Propose(ctx, ProposeRequest{
		System: followupSystem,
		User:   user,
		Force:  followupTool,
	})
	if err != nil {
		return FollowUp{}, err
	}

	var fu FollowUp
	if err := json.Unmarshal(out, &fu); err != nil {
		return FollowUp{}, fmt.Errorf("smarts: parse follow-up: %w", err)
	}
	if fu.Subject == "" || fu.Body == "" {
		return FollowUp{}, fmt.Errorf("smarts: empty follow-up draft")
	}
	return fu, nil
}
