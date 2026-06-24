package smarts

// Tool definitions handed to the model. Schemas are plain JSON-Schema objects
// ("properties" map + "required" string slice) consumed by toSDKTools.

// catalogueSearchTool is the shared read-only grounding capability: one search
// across all searchable catalogue fields (the repo scopes it to the tenant and
// the resolved version).
var catalogueSearchTool = Tool{
	Name:        "search",
	Description: "Search the price-list catalogue. Matches code, name, category, and unit. Returns matching items.",
	Schema: map[string]any{
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Words to search for across the catalogue (item name, code, category, or unit).",
			},
		},
		"required": []string{"query"},
	},
}

// draftInvoiceCommitTool is the forced final output of the draft-invoice Smart.
var draftInvoiceCommitTool = Tool{
	Name:        "draft_invoice",
	Description: "Emit the final invoice lines. One entry per distinct billable item, each with a catalogue code found via search.",
	Schema: map[string]any{
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code":        map[string]any{"type": "string", "description": "Catalogue code found via search."},
						"description": map[string]any{"type": "string", "description": "Line description for the invoice."},
						"unit":        map[string]any{"type": "string", "description": "Unit (defaults to the catalogue item's unit if omitted)."},
						"quantity":    map[string]any{"type": "number", "description": "Quantity (must be > 0)."},
						"serviceDate": map[string]any{"type": "string", "description": "ISO date (YYYY-MM-DD) the work was done."},
					},
					"required": []string{"code", "quantity"},
				},
			},
		},
		"required": []string{"items"},
	},
}

// suggestLinesCommitTool is the forced output of the suggest-lines Smart (same
// line shape as draft_invoice; no invoice is created).
var suggestLinesCommitTool = Tool{
	Name:        "suggest_lines",
	Description: "Emit the suggested line items, each with a catalogue code found via search.",
	Schema:      draftInvoiceCommitTool.Schema,
}

// followupTool is the forced output of the overdue follow-up Smart.
var followupTool = Tool{
	Name:        "draft_followup",
	Description: "Emit a polite payment-reminder email for an overdue invoice.",
	Schema: map[string]any{
		"properties": map[string]any{
			"subject": map[string]any{"type": "string", "description": "Email subject line."},
			"body":    map[string]any{"type": "string", "description": "Email body. Polite, factual, references the invoice number, amount, and due date."},
		},
		"required": []string{"subject", "body"},
	},
}

// mapColumnsTool is the forced output of the price-list import mapping Smart.
var mapColumnsTool = Tool{
	Name:        "map_columns",
	Description: "Map each source header to a target catalogue field. Omit headers that don't map to any target.",
	Schema: map[string]any{
		"properties": map[string]any{
			"mappings": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"header": map[string]any{"type": "string", "description": "A source column header from the file."},
						"field":  map[string]any{"type": "string", "description": "Target catalogue field this header maps to."},
					},
					"required": []string{"header", "field"},
				},
			},
		},
		"required": []string{"mappings"},
	},
}
