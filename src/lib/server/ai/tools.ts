import { repositories } from '$lib/repositories/index.js';
import type {
	CreateInvoiceInput,
	CreateClientInput,
	CreateEstimateInput,
	CreatePayerInput,
	CreateCatalogItemInput,
	CreateRateTierInput,
	LineItemInput
} from '$lib/repositories/index.js';

export type ToolKind = 'read' | 'write';

export interface ToolSpec {
	name: string;
	description: string;
	kind: ToolKind;
	paramSchema: Record<string, unknown>;
	execute(args: Record<string, unknown>): Promise<unknown>;
}

function asString(v: unknown, max = 200): string {
	return typeof v === 'string' ? v.slice(0, max) : '';
}
function asNumber(v: unknown, fallback = 0): number {
	const n = typeof v === 'number' ? v : Number(v);
	return Number.isFinite(n) ? n : fallback;
}
function asIdArray(v: unknown): number[] {
	if (!Array.isArray(v)) return [];
	const out: number[] = [];
	for (const item of v) {
		const n = asNumber(item, 0);
		if (n > 0) out.push(n);
		if (out.length >= 200) break;
	}
	return out;
}
function todayStr(): string {
	return new Date().toISOString().slice(0, 10);
}
function buildLineItems(raw: unknown): { items: LineItemInput[]; subtotal: number } {
	const arr = Array.isArray(raw) ? (raw as unknown[]) : [];
	if (arr.length === 0) throw new Error('at least one line item required');
	if (arr.length > 100) throw new Error('too many line items');
	const items: LineItemInput[] = [];
	let subtotal = 0;
	for (let i = 0; i < arr.length; i++) {
		const r = arr[i] as Record<string, unknown>;
		const qty = asNumber(r['quantity'], 1);
		const rate = asNumber(r['rate'], 0);
		const amount = qty * rate;
		subtotal += amount;
		items.push({
			description: asString(r['description'], 500),
			quantity: qty,
			rate,
			amount,
			sort_order: i
		});
	}
	return { items, subtotal };
}

const tools: ToolSpec[] = [
	// ── meta ──────────────────────────────────────────────────────────
	{
		name: 'askUserChoice',
		description:
			'Ask the user a yes/no or multiple-choice question with clickable buttons. After calling, stop and wait for the user reply.',
		kind: 'read',
		paramSchema: {
			type: 'object',
			properties: {
				question: { type: 'string' },
				options: { type: 'array', items: { type: 'string' } }
			},
			required: ['question', 'options']
		},
		execute: async (args) => {
			const question = asString(args['question'], 500);
			const raw = Array.isArray(args['options']) ? (args['options'] as unknown[]) : [];
			const options = raw.slice(0, 6).map((o) => asString(o, 80)).filter((o) => o.length > 0);
			if (!question || options.length === 0) throw new Error('question and options required');
			return { question, options, message: 'Awaiting user choice' };
		}
	},
	{
		name: 'getDashboardStats',
		description: 'Dashboard summary: total revenue, outstanding amount, overdue count, recent invoices, etc.',
		kind: 'read',
		paramSchema: { type: 'object', properties: {} },
		execute: async () => repositories.dashboard.getDashboardStats()
	},
	{
		name: 'getMonthlyRevenue',
		description: 'Last 12 months of paid revenue, default currency only.',
		kind: 'read',
		paramSchema: { type: 'object', properties: {} },
		execute: async () => repositories.dashboard.getMonthlyRevenue()
	},

	// ── clients ───────────────────────────────────────────────────────
	{
		name: 'searchClients',
		description: 'Search clients by name, email, or phone. Returns up to 20 matches.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { query: { type: 'string' } } },
		execute: async (args) => {
			const result = await repositories.clients.getClients(asString(args['query']), { page: 1, limit: 20 });
			return result.data.map((c) => ({ id: c.id, name: c.name, email: c.email, phone: c.phone }));
		}
	},
	{
		name: 'getClient',
		description: 'Get one client by id',
		kind: 'read',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => repositories.clients.getClient(asNumber(args['id']))
	},
	{
		name: 'getClientRevenue',
		description: 'Get total revenue, invoice count, and last invoice date for a client.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => repositories.clients.getClientRevenueSummary(asNumber(args['id']))
	},
	{
		name: 'createClient',
		description: 'Create a new client. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				name: { type: 'string' },
				email: { type: 'string' },
				phone: { type: 'string' },
				address: { type: 'string' }
			},
			required: ['name']
		},
		execute: async (args) => {
			const input: CreateClientInput = {
				name: asString(args['name']),
				email: asString(args['email']),
				phone: asString(args['phone']),
				address: asString(args['address'], 500)
			};
			if (!input.name) throw new Error('name required');
			const existing = await repositories.clients.getClients(input.name, { page: 1, limit: 5 });
			const match = existing.data.find(
				(c) => c.name.trim().toLowerCase() === input.name.trim().toLowerCase()
			);
			if (match) return { id: match.id, existing: true, message: `Client "${match.name}" already exists` };
			const id = await repositories.clients.createClient(input);
			return { id, existing: false };
		}
	},
	{
		name: 'updateClient',
		description: 'Update an existing client. Requires user approval. All fields besides id are optional; provided fields replace existing values.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				id: { type: 'number' },
				name: { type: 'string' },
				email: { type: 'string' },
				phone: { type: 'string' },
				address: { type: 'string' }
			},
			required: ['id']
		},
		execute: async (args) => {
			const id = asNumber(args['id']);
			const current = await repositories.clients.getClient(id);
			if (!current) throw new Error('client not found');
			await repositories.clients.updateClient(id, {
				name: asString(args['name']) || current.name,
				email: asString(args['email']) || current.email,
				phone: asString(args['phone']) || current.phone,
				address: asString(args['address'], 500) || current.address
			});
			return { id, updated: true };
		}
	},
	{
		name: 'deleteClient',
		description: 'Delete a client. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			const id = asNumber(args['id']);
			await repositories.clients.deleteClient(id);
			return { id, deleted: true };
		}
	},
	{
		name: 'bulkDeleteClients',
		description: 'Delete multiple clients at once. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: { ids: { type: 'array', items: { type: 'number' } } },
			required: ['ids']
		},
		execute: async (args) => {
			const ids = asIdArray(args['ids']);
			if (ids.length === 0) throw new Error('ids required');
			await repositories.clients.bulkDeleteClients(ids);
			return { count: ids.length, deleted: true };
		}
	},

	// ── invoices ──────────────────────────────────────────────────────
	{
		name: 'listInvoices',
		description: 'List invoices, optionally filtered by status (draft/sent/paid/overdue) or search.',
		kind: 'read',
		paramSchema: {
			type: 'object',
			properties: { status: { type: 'string' }, search: { type: 'string' } }
		},
		execute: async (args) => {
			const result = await repositories.invoices.getInvoices(
				asString(args['search']) || undefined,
				asString(args['status']) || undefined,
				{ page: 1, limit: 20 }
			);
			return result.data.map((i) => ({
				id: i.id,
				invoice_number: i.invoice_number,
				client_id: i.client_id,
				date: i.date,
				due_date: i.due_date,
				total: i.total,
				status: i.status
			}));
		}
	},
	{
		name: 'getInvoice',
		description: 'Get one invoice with line items by id.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			const id = asNumber(args['id']);
			const inv = await repositories.invoices.getInvoice(id);
			if (!inv) return null;
			const lines = await repositories.invoices.getInvoiceLineItems(id);
			return { ...inv, line_items: lines };
		}
	},
	{
		name: 'getClientInvoices',
		description: 'Get all invoices for one client.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { client_id: { type: 'number' } }, required: ['client_id'] },
		execute: async (args) => repositories.invoices.getClientInvoices(asNumber(args['client_id']))
	},
	{
		name: 'getAgingReport',
		description: 'Aging report: outstanding amounts grouped by 0-30, 31-60, 61-90, 90+ days overdue.',
		kind: 'read',
		paramSchema: { type: 'object', properties: {} },
		execute: async () => repositories.invoices.getAgingReport()
	},
	{
		name: 'createInvoice',
		description:
			'Create a new draft invoice. Requires user approval. Provide client_id (use searchClients first), date, due_date, line_items[]. Optional tax_rate.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				client_id: { type: 'number' },
				date: { type: 'string' },
				due_date: { type: 'string' },
				notes: { type: 'string' },
				tax_rate: { type: 'number' },
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
			required: ['client_id', 'line_items']
		},
		execute: async (args) => {
			const clientId = asNumber(args['client_id']);
			if (clientId <= 0) throw new Error('valid client_id required');
			const { items, subtotal } = buildLineItems(args['line_items']);
			const taxRate = asNumber(args['tax_rate'], 0);
			const taxAmount = (subtotal * taxRate) / 100;
			const date = asString(args['date'], 10) || todayStr();
			const dueDate = asString(args['due_date'], 10) || date;
			const recent = await repositories.invoices.getInvoices(undefined, undefined, { page: 1, limit: 30 });
			const cutoff = Date.now() - 5 * 60 * 1000;
			const dup = recent.data.find(
				(inv) =>
					inv.client_id === clientId &&
					Math.abs(inv.total - (subtotal + taxAmount)) < 0.01 &&
					inv.date === date &&
					inv.due_date === dueDate &&
					new Date(inv.created_at).getTime() > cutoff
			);
			if (dup) return { id: dup.id, invoice_number: dup.invoice_number, total: dup.total, existing: true };
			const invoiceNumber = `AI-${Date.now()}`;
			const input: CreateInvoiceInput = {
				invoice_number: invoiceNumber,
				client_id: clientId,
				date,
				due_date: dueDate,
				subtotal,
				tax_rate: taxRate,
				tax_amount: taxAmount,
				total: subtotal + taxAmount,
				notes: asString(args['notes'], 1000),
				status: 'draft'
			};
			const id = await repositories.invoices.createInvoice(input, items);
			return { id, invoice_number: invoiceNumber, total: input.total, existing: false };
		}
	},
	{
		name: 'updateInvoiceStatus',
		description: 'Set invoice status (draft, sent, paid, overdue, cancelled). Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: { id: { type: 'number' }, status: { type: 'string' } },
			required: ['id', 'status']
		},
		execute: async (args) => {
			const id = asNumber(args['id']);
			const status = asString(args['status'], 20);
			await repositories.invoices.updateInvoiceStatus(id, status);
			return { id, status, updated: true };
		}
	},
	{
		name: 'deleteInvoice',
		description: 'Delete an invoice. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			const id = asNumber(args['id']);
			await repositories.invoices.deleteInvoice(id);
			return { id, deleted: true };
		}
	},
	{
		name: 'duplicateInvoice',
		description: 'Duplicate an existing invoice as a new draft. Requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			const newId = await repositories.invoices.duplicateInvoice(asNumber(args['id']));
			return { newId, duplicated: true };
		}
	},
	{
		name: 'bulkDeleteInvoices',
		description: 'Delete multiple invoices. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: { ids: { type: 'array', items: { type: 'number' } } },
			required: ['ids']
		},
		execute: async (args) => {
			const ids = asIdArray(args['ids']);
			if (ids.length === 0) throw new Error('ids required');
			await repositories.invoices.bulkDeleteInvoices(ids);
			return { count: ids.length, deleted: true };
		}
	},
	{
		name: 'bulkUpdateInvoiceStatus',
		description: 'Set status on multiple invoices. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				ids: { type: 'array', items: { type: 'number' } },
				status: { type: 'string' }
			},
			required: ['ids', 'status']
		},
		execute: async (args) => {
			const ids = asIdArray(args['ids']);
			const status = asString(args['status'], 20);
			if (ids.length === 0 || !status) throw new Error('ids and status required');
			await repositories.invoices.bulkUpdateInvoiceStatus(ids, status);
			return { count: ids.length, status, updated: true };
		}
	},

	// ── estimates ─────────────────────────────────────────────────────
	{
		name: 'listEstimates',
		description: 'List estimates, optionally filtered by status or search.',
		kind: 'read',
		paramSchema: {
			type: 'object',
			properties: { status: { type: 'string' }, search: { type: 'string' } }
		},
		execute: async (args) => {
			const result = await repositories.estimates.getEstimates(
				asString(args['search']) || undefined,
				asString(args['status']) || undefined,
				{ page: 1, limit: 20 }
			);
			return result.data.map((e) => ({
				id: e.id,
				estimate_number: e.estimate_number,
				client_id: e.client_id,
				date: e.date,
				valid_until: e.valid_until,
				total: e.total,
				status: e.status
			}));
		}
	},
	{
		name: 'getEstimate',
		description: 'Get one estimate with line items by id.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			const id = asNumber(args['id']);
			const est = await repositories.estimates.getEstimate(id);
			if (!est) return null;
			const lines = await repositories.estimates.getEstimateLineItems(id);
			return { ...est, line_items: lines };
		}
	},
	{
		name: 'createEstimate',
		description: 'Create a new draft estimate. Requires user approval. Provide client_id, valid_until, line_items[].',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				client_id: { type: 'number' },
				date: { type: 'string' },
				valid_until: { type: 'string' },
				notes: { type: 'string' },
				tax_rate: { type: 'number' },
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
			required: ['client_id', 'line_items']
		},
		execute: async (args) => {
			const clientId = asNumber(args['client_id']);
			if (clientId <= 0) throw new Error('valid client_id required');
			const { items, subtotal } = buildLineItems(args['line_items']);
			const taxRate = asNumber(args['tax_rate'], 0);
			const taxAmount = (subtotal * taxRate) / 100;
			const date = asString(args['date'], 10) || todayStr();
			const validUntil = asString(args['valid_until'], 10) || date;
			const input: CreateEstimateInput = {
				estimate_number: `AI-EST-${Date.now()}`,
				client_id: clientId,
				date,
				valid_until: validUntil,
				subtotal,
				tax_rate: taxRate,
				tax_amount: taxAmount,
				total: subtotal + taxAmount,
				notes: asString(args['notes'], 1000),
				status: 'draft'
			};
			const id = await repositories.estimates.createEstimate(input, items);
			return { id, estimate_number: input.estimate_number, total: input.total };
		}
	},
	{
		name: 'updateEstimateStatus',
		description: 'Set estimate status (draft, sent, accepted, declined, expired). Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: { id: { type: 'number' }, status: { type: 'string' } },
			required: ['id', 'status']
		},
		execute: async (args) => {
			const id = asNumber(args['id']);
			const status = asString(args['status'], 20);
			await repositories.estimates.updateEstimateStatus(id, status);
			return { id, status, updated: true };
		}
	},
	{
		name: 'deleteEstimate',
		description: 'Delete an estimate. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			await repositories.estimates.deleteEstimate(asNumber(args['id']));
			return { id: asNumber(args['id']), deleted: true };
		}
	},
	{
		name: 'convertEstimateToInvoice',
		description: 'Convert an accepted estimate into a new invoice. Requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			const newId = await repositories.estimates.convertEstimateToInvoice(asNumber(args['id']));
			return { invoice_id: newId, converted: true };
		}
	},

	// ── payments ──────────────────────────────────────────────────────
	{
		name: 'getInvoicePayments',
		description: 'List payments recorded against one invoice.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { invoice_id: { type: 'number' } }, required: ['invoice_id'] },
		execute: async (args) => repositories.payments.getInvoicePayments(asNumber(args['invoice_id']))
	},
	{
		name: 'getInvoiceTotalPaid',
		description: 'Sum of all payments recorded against one invoice.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { invoice_id: { type: 'number' } }, required: ['invoice_id'] },
		execute: async (args) => ({
			total_paid: await repositories.payments.getInvoiceTotalPaid(asNumber(args['invoice_id']))
		})
	},
	{
		name: 'recordPayment',
		description: 'Record a payment against an invoice. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				invoice_id: { type: 'number' },
				amount: { type: 'number' },
				payment_date: { type: 'string' },
				method: { type: 'string' },
				notes: { type: 'string' }
			},
			required: ['invoice_id', 'amount']
		},
		execute: async (args) => {
			const id = await repositories.payments.createPayment({
				invoice_id: asNumber(args['invoice_id']),
				amount: asNumber(args['amount']),
				payment_date: asString(args['payment_date'], 10) || todayStr(),
				method: asString(args['method'], 50),
				notes: asString(args['notes'], 500)
			});
			return { id, recorded: true };
		}
	},
	{
		name: 'deletePayment',
		description: 'Delete a recorded payment. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			await repositories.payments.deletePayment(asNumber(args['id']));
			return { id: asNumber(args['id']), deleted: true };
		}
	},

	// ── catalog ───────────────────────────────────────────────────────
	{
		name: 'searchCatalog',
		description: 'Search catalog items by name or sku.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { query: { type: 'string' } } },
		execute: async (args) => {
			const result = await repositories.catalog.getCatalogItems(asString(args['query']), undefined, { page: 1, limit: 20 });
			return result.data.map((c) => ({ id: c.id, name: c.name, rate: c.rate, unit: c.unit, sku: c.sku }));
		}
	},
	{
		name: 'getCatalogItem',
		description: 'Get one catalog item by id.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => repositories.catalog.getCatalogItem(asNumber(args['id']))
	},
	{
		name: 'createCatalogItem',
		description: 'Add a new catalog item. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				name: { type: 'string' },
				rate: { type: 'number' },
				unit: { type: 'string' },
				category: { type: 'string' },
				sku: { type: 'string' }
			},
			required: ['name']
		},
		execute: async (args) => {
			const input: CreateCatalogItemInput = {
				name: asString(args['name']),
				rate: asNumber(args['rate'], 0),
				unit: asString(args['unit'], 30),
				category: asString(args['category'], 100),
				sku: asString(args['sku'], 100)
			};
			if (!input.name) throw new Error('name required');
			const id = await repositories.catalog.createCatalogItem(input);
			return { id };
		}
	},
	{
		name: 'updateCatalogItem',
		description: 'Update a catalog item. Requires user approval. Provided fields replace existing values.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				id: { type: 'number' },
				name: { type: 'string' },
				rate: { type: 'number' },
				unit: { type: 'string' },
				category: { type: 'string' },
				sku: { type: 'string' }
			},
			required: ['id']
		},
		execute: async (args) => {
			const id = asNumber(args['id']);
			const current = await repositories.catalog.getCatalogItem(id);
			if (!current) throw new Error('catalog item not found');
			await repositories.catalog.updateCatalogItem(id, {
				name: asString(args['name']) || current.name,
				rate: typeof args['rate'] === 'number' ? Number(args['rate']) : current.rate,
				unit: asString(args['unit'], 30) || current.unit,
				category: asString(args['category'], 100) || current.category,
				sku: asString(args['sku'], 100) || current.sku
			});
			return { id, updated: true };
		}
	},
	{
		name: 'deleteCatalogItem',
		description: 'Delete a catalog item. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			await repositories.catalog.deleteCatalogItem(asNumber(args['id']));
			return { id: asNumber(args['id']), deleted: true };
		}
	},

	// ── payers ────────────────────────────────────────────────────────
	{
		name: 'listPayers',
		description: 'Search payers (entities that pay invoices on behalf of clients).',
		kind: 'read',
		paramSchema: { type: 'object', properties: { query: { type: 'string' } } },
		execute: async (args) => {
			const items = await repositories.payers.getPayers(asString(args['query']));
			return items.slice(0, 20).map((p) => ({ id: p.id, name: p.name, email: p.email }));
		}
	},
	{
		name: 'getPayer',
		description: 'Get one payer by id.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => repositories.payers.getPayer(asNumber(args['id']))
	},
	{
		name: 'createPayer',
		description: 'Create a new payer. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				name: { type: 'string' },
				email: { type: 'string' },
				phone: { type: 'string' },
				address: { type: 'string' }
			},
			required: ['name']
		},
		execute: async (args) => {
			const input: CreatePayerInput = {
				name: asString(args['name']),
				email: asString(args['email']),
				phone: asString(args['phone']),
				address: asString(args['address'], 500)
			};
			if (!input.name) throw new Error('name required');
			const id = await repositories.payers.createPayer(input);
			return { id };
		}
	},
	{
		name: 'updatePayer',
		description: 'Update a payer. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				id: { type: 'number' },
				name: { type: 'string' },
				email: { type: 'string' },
				phone: { type: 'string' },
				address: { type: 'string' }
			},
			required: ['id']
		},
		execute: async (args) => {
			const id = asNumber(args['id']);
			const current = await repositories.payers.getPayer(id);
			if (!current) throw new Error('payer not found');
			await repositories.payers.updatePayer(id, {
				name: asString(args['name']) || current.name,
				email: asString(args['email']) || current.email,
				phone: asString(args['phone']) || current.phone,
				address: asString(args['address'], 500) || current.address
			});
			return { id, updated: true };
		}
	},
	{
		name: 'deletePayer',
		description: 'Delete a payer. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			await repositories.payers.deletePayer(asNumber(args['id']));
			return { id: asNumber(args['id']), deleted: true };
		}
	},

	// ── tax rates ─────────────────────────────────────────────────────
	{
		name: 'listTaxRates',
		description: 'List all configured tax rates.',
		kind: 'read',
		paramSchema: { type: 'object', properties: {} },
		execute: async () => repositories.taxRates.getTaxRates()
	},
	{
		name: 'createTaxRate',
		description: 'Create a tax rate. Requires user approval. Rate is a percent (e.g. 10 for 10%).',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				name: { type: 'string' },
				rate: { type: 'number' },
				is_default: { type: 'boolean' }
			},
			required: ['name', 'rate']
		},
		execute: async (args) => {
			const id = await repositories.taxRates.createTaxRate({
				name: asString(args['name'], 100),
				rate: asNumber(args['rate']),
				is_default: args['is_default'] === true
			});
			return { id };
		}
	},
	{
		name: 'updateTaxRate',
		description: 'Update a tax rate. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				id: { type: 'number' },
				name: { type: 'string' },
				rate: { type: 'number' },
				is_default: { type: 'boolean' }
			},
			required: ['id', 'name', 'rate']
		},
		execute: async (args) => {
			const id = asNumber(args['id']);
			await repositories.taxRates.updateTaxRate(id, {
				name: asString(args['name'], 100),
				rate: asNumber(args['rate']),
				is_default: args['is_default'] === true
			});
			return { id, updated: true };
		}
	},
	{
		name: 'deleteTaxRate',
		description: 'Delete a tax rate. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			await repositories.taxRates.deleteTaxRate(asNumber(args['id']));
			return { id: asNumber(args['id']), deleted: true };
		}
	},

	// ── rate tiers ────────────────────────────────────────────────────
	{
		name: 'listRateTiers',
		description: 'List all rate tiers (pricing levels for catalog items).',
		kind: 'read',
		paramSchema: { type: 'object', properties: {} },
		execute: async () => repositories.rateTiers.getRateTiers()
	},
	{
		name: 'createRateTier',
		description: 'Create a rate tier. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				name: { type: 'string' },
				description: { type: 'string' },
				sort_order: { type: 'number' }
			},
			required: ['name']
		},
		execute: async (args) => {
			const input: CreateRateTierInput = {
				name: asString(args['name'], 100),
				description: asString(args['description'], 500),
				sort_order: asNumber(args['sort_order'], 0)
			};
			const id = await repositories.rateTiers.createRateTier(input);
			return { id };
		}
	},
	{
		name: 'deleteRateTier',
		description: 'Delete a rate tier. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			await repositories.rateTiers.deleteRateTier(asNumber(args['id']));
			return { id: asNumber(args['id']), deleted: true };
		}
	},

	// ── recurring templates ───────────────────────────────────────────
	{
		name: 'listRecurringTemplates',
		description: 'List recurring invoice templates. activeOnly=true filters to active templates.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { activeOnly: { type: 'boolean' } } },
		execute: async (args) =>
			repositories.recurringTemplates.getRecurringTemplates(args['activeOnly'] === true)
	},
	{
		name: 'getRecurringTemplate',
		description: 'Get one recurring template by id.',
		kind: 'read',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => repositories.recurringTemplates.getRecurringTemplate(asNumber(args['id']))
	},
	{
		name: 'deleteRecurringTemplate',
		description: 'Delete a recurring template. Destructive — requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			await repositories.recurringTemplates.deleteRecurringTemplate(asNumber(args['id']));
			return { id: asNumber(args['id']), deleted: true };
		}
	},
	{
		name: 'runRecurringTemplate',
		description: 'Generate an invoice from a recurring template now and advance next_due. Requires user approval.',
		kind: 'write',
		paramSchema: { type: 'object', properties: { id: { type: 'number' } }, required: ['id'] },
		execute: async (args) => {
			const invoiceId = await repositories.recurringTemplates.createInvoiceFromTemplate(asNumber(args['id']));
			return { invoice_id: invoiceId };
		}
	},

	// ── business profile ──────────────────────────────────────────────
	{
		name: 'getBusinessProfile',
		description: 'Get the business profile (company name, address, default currency, etc).',
		kind: 'read',
		paramSchema: { type: 'object', properties: {} },
		execute: async () => repositories.businessProfile.getBusinessProfile()
	},
	{
		name: 'saveBusinessProfile',
		description: 'Save business profile. Requires user approval.',
		kind: 'write',
		paramSchema: {
			type: 'object',
			properties: {
				name: { type: 'string' },
				email: { type: 'string' },
				phone: { type: 'string' },
				address: { type: 'string' },
				default_currency: { type: 'string' }
			},
			required: ['name']
		},
		execute: async (args) => {
			await repositories.businessProfile.saveBusinessProfile({
				name: asString(args['name']),
				email: asString(args['email']),
				phone: asString(args['phone']),
				address: asString(args['address'], 500),
				default_currency: asString(args['default_currency'], 10)
			});
			return { saved: true };
		}
	}
];

const toolMap = new Map<string, ToolSpec>(tools.map((t) => [t.name, t]));

export function listTools(): ToolSpec[] {
	return tools;
}

export function getTool(name: string): ToolSpec | undefined {
	return toolMap.get(name);
}
