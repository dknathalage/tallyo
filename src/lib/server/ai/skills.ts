export interface Skill {
	id: string;
	title: string;
	description: string;
	tools: string[];
	promptAddendum?: string;
	routes?: RegExp[];
	keywords?: string[];
	alwaysLoaded?: boolean;
}

const SKILLS: Skill[] = [
	{
		id: 'core',
		title: 'Core',
		description: 'Always-on capabilities: dashboard, business profile, ask the user choices, manage skills, delegate to specialized agents.',
		tools: [
			'askUserChoice',
			'getDashboardStats',
			'getMonthlyRevenue',
			'getBusinessProfile',
			'loadSkill',
			'listAvailableSkills',
			'delegateToInvoiceAgent',
			'delegateToCollectionAgent',
			'delegateToCatalogBulkAgent'
		],
		alwaysLoaded: true,
		promptAddendum: 'For complex multi-step domain tasks, prefer delegating to a specialized agent.'
	},
	{
		id: 'clients',
		title: 'Clients',
		description: 'Search, view, and manage client records.',
		tools: [
			'searchClients',
			'getClient',
			'getClientRevenue',
			'createClient',
			'updateClient',
			'deleteClient',
			'bulkDeleteClients'
		],
		routes: [/^\/console\/clients/],
		keywords: ['client', 'customer', 'contact']
	},
	{
		id: 'invoices',
		title: 'Invoices',
		description: 'List, create, update, delete invoices and view aging.',
		tools: [
			'listInvoices',
			'getInvoice',
			'getClientInvoices',
			'getAgingReport',
			'createInvoice',
			'updateInvoiceStatus',
			'deleteInvoice',
			'duplicateInvoice',
			'bulkDeleteInvoices',
			'bulkUpdateInvoiceStatus'
		],
		routes: [/^\/console\/invoices/],
		keywords: ['invoice', 'bill', 'charge']
	},
	{
		id: 'estimates',
		title: 'Estimates',
		description: 'Manage quotes/estimates and convert accepted ones to invoices.',
		tools: [
			'listEstimates',
			'getEstimate',
			'createEstimate',
			'updateEstimateStatus',
			'deleteEstimate',
			'convertEstimateToInvoice'
		],
		routes: [/^\/console\/estimates/],
		keywords: ['estimate', 'quote', 'proposal']
	},
	{
		id: 'payments',
		title: 'Payments',
		description: 'Record and inspect payments against invoices.',
		tools: ['getInvoicePayments', 'getInvoiceTotalPaid', 'recordPayment', 'deletePayment'],
		routes: [/^\/console\/invoices\/[^/]+/],
		keywords: ['payment', 'paid', 'received']
	},
	{
		id: 'catalog',
		title: 'Catalog',
		description: 'Manage products and services catalog.',
		tools: [
			'searchCatalog',
			'getCatalogItem',
			'createCatalogItem',
			'updateCatalogItem',
			'deleteCatalogItem'
		],
		routes: [/^\/console\/catalog/],
		keywords: ['catalog', 'item', 'product', 'service']
	},
	{
		id: 'payers',
		title: 'Payers',
		description: 'Manage payers (entities that pay invoices on behalf of clients).',
		tools: ['listPayers', 'getPayer', 'createPayer', 'updatePayer', 'deletePayer'],
		routes: [/^\/console\/payers/],
		keywords: ['payer']
	},
	{
		id: 'tax',
		title: 'Tax rates',
		description: 'Configure tax rates.',
		tools: ['listTaxRates', 'createTaxRate', 'updateTaxRate', 'deleteTaxRate'],
		routes: [/^\/console\/settings/],
		keywords: ['tax']
	},
	{
		id: 'tiers',
		title: 'Rate tiers',
		description: 'Manage pricing tiers for catalog items.',
		tools: ['listRateTiers', 'createRateTier', 'deleteRateTier'],
		routes: [/^\/console\/rate-tiers/],
		keywords: ['rate tier', 'pricing']
	},
	{
		id: 'recurring',
		title: 'Recurring',
		description: 'Recurring invoice templates: list, run now, delete.',
		tools: [
			'listRecurringTemplates',
			'getRecurringTemplate',
			'deleteRecurringTemplate',
			'runRecurringTemplate'
		],
		routes: [/^\/console\/recurring/],
		keywords: ['recurring', 'subscription', 'monthly']
	},
	{
		id: 'business',
		title: 'Business profile',
		description: 'View and update the business profile.',
		tools: ['saveBusinessProfile'],
		routes: [/^\/console\/settings/],
		keywords: ['business profile', 'company']
	}
];

const skillMap = new Map<string, Skill>(SKILLS.map((s) => [s.id, s]));

export function listSkills(): Skill[] {
	return SKILLS;
}

export function getSkill(id: string): Skill | undefined {
	return skillMap.get(id);
}

export interface ResolveOpts {
	route?: string;
	userMessage?: string;
	explicitlyLoaded?: string[];
	cap?: number;
}

export function resolveSkillsForContext(opts: ResolveOpts): Skill[] {
	const cap = opts.cap ?? 4;
	const out = new Map<string, Skill>();

	for (const s of SKILLS) {
		if (s.alwaysLoaded) out.set(s.id, s);
	}

	const remaining = (): number => cap - countNonAlwaysLoaded(out);

	for (const id of opts.explicitlyLoaded ?? []) {
		if (out.has(id)) continue;
		const s = skillMap.get(id);
		if (!s) continue;
		if (remaining() <= 0) break;
		out.set(id, s);
	}

	if (opts.route) {
		for (const s of SKILLS) {
			if (out.has(s.id)) continue;
			if (!s.routes || s.routes.length === 0) continue;
			if (s.routes.some((r) => r.test(opts.route!))) {
				if (remaining() <= 0) break;
				out.set(s.id, s);
			}
		}
	}

	if (opts.userMessage) {
		const lower = opts.userMessage.toLowerCase();
		for (const s of SKILLS) {
			if (out.has(s.id)) continue;
			if (!s.keywords || s.keywords.length === 0) continue;
			if (s.keywords.some((k) => lower.includes(k.toLowerCase()))) {
				if (remaining() <= 0) break;
				out.set(s.id, s);
			}
		}
	}

	return Array.from(out.values());
}

function countNonAlwaysLoaded(map: Map<string, Skill>): number {
	let n = 0;
	for (const s of map.values()) if (!s.alwaysLoaded) n++;
	return n;
}
