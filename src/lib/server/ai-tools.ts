import Anthropic from '@anthropic-ai/sdk';
import { repositories } from '$lib/repositories/index.js';

export const SYSTEM_PROMPT = `You are an AI assistant built into an invoice management application. You help users manage their business finances through natural conversation.

Core principles:
1. EXPLORE FIRST: Always read current data before making changes. Never guess at IDs — always list first.
2. PLAN: For multi-step tasks, briefly state what you will do before doing it.
3. EXECUTE carefully: Make minimal, targeted changes.
4. VALIDATE: After any write operation, confirm by reading back the result.
5. RETRY: If a tool returns an error, diagnose and try a corrected approach.

You have full access to invoices, estimates, clients, payments, catalog items, and financial reports. Be helpful, concise, and proactive about surfacing useful insights.`;

export const AI_TOOLS: Anthropic.Tool[] = [
	{
		name: 'list_invoices',
		description: 'List invoices. Always call this before referencing invoice IDs.',
		input_schema: {
			type: 'object' as const,
			properties: {
				search: { type: 'string', description: 'Search by invoice number or client name' },
				status: { type: 'string', enum: ['draft', 'sent', 'paid', 'overdue'] }
			}
		}
	},
	{
		name: 'get_invoice',
		description: 'Get full details of a specific invoice including line items and payments',
		input_schema: {
			type: 'object' as const,
			properties: { id: { type: 'number' } },
			required: ['id']
		}
	},
	{
		name: 'create_invoice',
		description:
			'Create a new invoice with line items. Always list clients first to get the client_id.',
		input_schema: {
			type: 'object' as const,
			properties: {
				client_id: { type: 'number' },
				currency_code: { type: 'string', description: '3-letter currency code e.g. USD, AUD' },
				date: { type: 'string', description: 'YYYY-MM-DD' },
				due_date: { type: 'string', description: 'YYYY-MM-DD' },
				notes: { type: 'string' },
				line_items: {
					type: 'array',
					items: {
						type: 'object',
						properties: {
							description: { type: 'string' },
							quantity: { type: 'number' },
							rate: { type: 'number' }
						},
						required: ['description', 'quantity', 'rate']
					}
				}
			},
			required: ['client_id', 'currency_code', 'date', 'due_date', 'line_items']
		}
	},
	{
		name: 'update_invoice_status',
		description: 'Update the status of an invoice',
		input_schema: {
			type: 'object' as const,
			properties: {
				id: { type: 'number' },
				status: { type: 'string', enum: ['draft', 'sent', 'paid', 'overdue'] }
			},
			required: ['id', 'status']
		}
	},
	{
		name: 'list_clients',
		description: 'List clients. Always call this before referencing client IDs.',
		input_schema: {
			type: 'object' as const,
			properties: { search: { type: 'string' } }
		}
	},
	{
		name: 'get_client',
		description: 'Get client details and revenue summary',
		input_schema: {
			type: 'object' as const,
			properties: { id: { type: 'number' } },
			required: ['id']
		}
	},
	{
		name: 'create_client',
		description: 'Create a new client',
		input_schema: {
			type: 'object' as const,
			properties: {
				name: { type: 'string' },
				email: { type: 'string' },
				phone: { type: 'string' },
				address: { type: 'string' }
			},
			required: ['name']
		}
	},
	{
		name: 'get_dashboard_stats',
		description:
			'Get dashboard statistics: total revenue, outstanding, overdue count, recent activity',
		input_schema: { type: 'object' as const, properties: {} }
	},
	{
		name: 'get_aging_report',
		description: 'Get aging report showing overdue invoices by age bucket',
		input_schema: { type: 'object' as const, properties: {} }
	},
	{
		name: 'list_estimates',
		description: 'List estimates with optional filters',
		input_schema: {
			type: 'object' as const,
			properties: {
				search: { type: 'string' },
				status: { type: 'string' }
			}
		}
	},
	{
		name: 'record_payment',
		description: 'Record a payment against an invoice',
		input_schema: {
			type: 'object' as const,
			properties: {
				invoice_id: { type: 'number' },
				amount: { type: 'number' },
				payment_date: { type: 'string', description: 'YYYY-MM-DD' },
				method: { type: 'string' },
				notes: { type: 'string' }
			},
			required: ['invoice_id', 'amount', 'payment_date']
		}
	},
	{
		name: 'search_catalog',
		description: 'Search catalog items for use in invoices',
		input_schema: {
			type: 'object' as const,
			properties: {
				term: { type: 'string' },
				limit: { type: 'number' }
			},
			required: ['term']
		}
	}
];

async function toolListInvoices(input: Record<string, unknown>): Promise<unknown> {
	await repositories.invoices.markOverdueInvoices();
	return repositories.invoices.getInvoices(
		input['search'] as string | undefined,
		input['status'] as string | undefined
	);
}

async function toolGetInvoice(input: Record<string, unknown>): Promise<unknown> {
	const id = input['id'] as number;
	const invoice = await repositories.invoices.getInvoice(id);
	if (invoice === null) return { error: `Invoice ${String(id)} not found` };
	const lineItems = await repositories.invoices.getInvoiceLineItems(id);
	const payments = await repositories.payments.getInvoicePayments(id);
	return { ...invoice, line_items: lineItems, payments };
}

async function toolCreateInvoice(input: Record<string, unknown>): Promise<unknown> {
	const rawItems =
		(input['line_items'] as
			| { description: string; quantity: number; rate: number }[]
			| undefined) ?? [];
	const lineItems = rawItems.map((item, idx) => ({
		description: item.description,
		quantity: item.quantity,
		rate: item.rate,
		amount: item.quantity * item.rate,
		sort_order: idx,
		notes: ''
	}));
	const subtotal = lineItems.reduce((sum, li) => sum + li.amount, 0);
	const invoiceData = {
		invoice_number: `INV-${String(Date.now())}`,
		client_id: input['client_id'] as number,
		date: input['date'] as string,
		due_date: input['due_date'] as string,
		currency_code: (input['currency_code'] as string | undefined) ?? 'USD',
		notes: (input['notes'] as string | undefined) ?? '',
		subtotal,
		tax_rate: 0,
		tax_amount: 0,
		total: subtotal,
		status: 'draft'
	};
	const id = await repositories.invoices.createInvoice(invoiceData, lineItems);
	return { id, message: `Invoice created with ID ${String(id)}` };
}

async function toolUpdateInvoiceStatus(input: Record<string, unknown>): Promise<unknown> {
	const id = input['id'] as number;
	const status = input['status'] as string;
	await repositories.invoices.updateInvoiceStatus(id, status);
	return { success: true, message: `Invoice ${String(id)} updated to ${status}` };
}

async function toolGetClient(input: Record<string, unknown>): Promise<unknown> {
	const id = input['id'] as number;
	const client = await repositories.clients.getClient(id);
	if (client === null) return { error: `Client ${String(id)} not found` };
	const revenue = await repositories.clients.getClientRevenueSummary(id);
	return { ...client, revenue_summary: revenue };
}

async function toolCreateClient(input: Record<string, unknown>): Promise<unknown> {
	const email = input['email'] as string | undefined;
	const phone = input['phone'] as string | undefined;
	const address = input['address'] as string | undefined;
	const id = await repositories.clients.createClient({
		name: input['name'] as string,
		...(email !== undefined && { email }),
		...(phone !== undefined && { phone }),
		...(address !== undefined && { address })
	});
	return { id, message: `Client created with ID ${String(id)}` };
}

async function toolRecordPayment(input: Record<string, unknown>): Promise<unknown> {
	const method = input['method'] as string | undefined;
	const notes = input['notes'] as string | undefined;
	const id = await repositories.payments.createPayment({
		invoice_id: input['invoice_id'] as number,
		amount: input['amount'] as number,
		payment_date: input['payment_date'] as string,
		...(method !== undefined && { method }),
		...(notes !== undefined && { notes })
	});
	return { id, message: `Payment recorded` };
}

type ToolHandler = (input: Record<string, unknown>) => Promise<unknown>;

const toolHandlers: Record<string, ToolHandler> = {
	list_invoices: toolListInvoices,
	get_invoice: toolGetInvoice,
	create_invoice: toolCreateInvoice,
	update_invoice_status: toolUpdateInvoiceStatus,
	list_clients: (input) => repositories.clients.getClients(input['search'] as string | undefined),
	get_client: toolGetClient,
	create_client: toolCreateClient,
	get_dashboard_stats: () => repositories.dashboard.getDashboardStats(),
	get_aging_report: () => repositories.invoices.getAgingReport(),
	list_estimates: (input) =>
		repositories.estimates.getEstimates(
			input['search'] as string | undefined,
			input['status'] as string | undefined
		),
	record_payment: toolRecordPayment,
	search_catalog: (input) =>
		repositories.catalog.searchCatalogItems(
			input['term'] as string,
			(input['limit'] as number | undefined) ?? 10
		)
};

export async function executeTool(
	name: string,
	input: Record<string, unknown>
): Promise<unknown> {
	const handler = toolHandlers[name];
	if (handler === undefined) {
		return { error: `Unknown tool: ${name}` };
	}
	try {
		return await handler(input);
	} catch (e) {
		return { error: e instanceof Error ? e.message : String(e) };
	}
}
