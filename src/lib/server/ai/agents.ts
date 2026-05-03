export interface SubAgent {
	id: string;
	title: string;
	description: string;
	systemPrompt: string;
	skills: string[];
	inputSchema: Record<string, unknown>;
	maxSteps?: number;
}

const AGENTS: SubAgent[] = [
	{
		id: 'invoice',
		title: 'Invoice agent',
		description:
			'Drafts an invoice end-to-end from notes or a description. Use for: "create invoice for X based on these notes", "bill the client for last week", "make an invoice from this timesheet".',
		systemPrompt: `You are the InvoiceAgent. Your single job is to draft ONE invoice based on the user task.

Workflow:
1. Identify the client by calling searchClients with the most distinctive name token. If multiple matches, prefer exact match. If none, ask for clarification (call askUserChoice).
2. For each line item mentioned in the task, optionally call searchCatalog to reuse existing catalog rates.
3. Assemble line items and call createInvoice once. The system will queue it for user approval.
4. Stop. Do not retry. Do not call multiple write tools.

Be concise. Only describe what you found and what you are queueing.`,
		skills: ['core', 'clients', 'invoices', 'catalog'],
		inputSchema: {
			type: 'object',
			properties: {
				task: { type: 'string' },
				hint: {
					type: 'object',
					properties: {
						client_name: { type: 'string' },
						line_items: { type: 'array' }
					}
				}
			},
			required: ['task']
		},
		maxSteps: 6
	},
	{
		id: 'collection',
		title: 'Collection agent',
		description:
			'Reviews overdue or aging invoices and suggests follow-up. Use for: "who owes us money", "which invoices are overdue", "follow up on unpaid".',
		systemPrompt: `You are the CollectionAgent. Your job is to summarize outstanding invoices.

Workflow:
1. Call getAgingReport to see buckets.
2. Call listInvoices with status="overdue" or status="sent" depending on focus.
3. For each overdue client (max 5), call getClient to get contact info and getInvoiceTotalPaid for context.
4. Produce a concise summary grouped by client. Optionally propose status updates (call updateInvoiceStatus once if appropriate).
5. Stop.`,
		skills: ['core', 'invoices', 'payments', 'clients'],
		inputSchema: {
			type: 'object',
			properties: { focus: { type: 'string', enum: ['overdue', 'aging', 'all'] } },
			required: []
		},
		maxSteps: 8
	},
	{
		id: 'catalog-bulk',
		title: 'Catalog bulk agent',
		description:
			'Performs bulk catalog edits matching a criteria. Use for: "rename all X items", "reprice category Y", "delete unused items".',
		systemPrompt: `You are the CatalogBulkAgent. You make bulk edits to catalog items.

Workflow:
1. Call searchCatalog with the criteria to find matching items.
2. Show the user the matched items first via askUserChoice — confirm scope before any write.
3. For each confirmed item, queue a single write per turn (updateCatalogItem or deleteCatalogItem). The system enforces one write per turn; the user will approve, then you continue.
4. Stop when done or when no items match.`,
		skills: ['core', 'catalog'],
		inputSchema: {
			type: 'object',
			properties: {
				operation: { type: 'string', enum: ['rename', 'reprice', 'categorize', 'delete'] },
				criteria: { type: 'string' }
			},
			required: ['operation', 'criteria']
		},
		maxSteps: 6
	}
];

const agentMap = new Map<string, SubAgent>(AGENTS.map((a) => [a.id, a]));

export function listAgents(): SubAgent[] {
	return AGENTS;
}

export function getAgent(id: string): SubAgent | undefined {
	return agentMap.get(id);
}

export const AGENT_TOOL_NAMES = AGENTS.map((a) => `delegateTo${capitalize(a.id.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase()))}Agent`);

function capitalize(s: string): string {
	return s.length === 0 ? s : s[0]!.toUpperCase() + s.slice(1);
}

export function agentToolName(agentId: string): string {
	const camel = agentId.replace(/-([a-z])/g, (_, c: string) => c.toUpperCase());
	return `delegateTo${capitalize(camel)}Agent`;
}
