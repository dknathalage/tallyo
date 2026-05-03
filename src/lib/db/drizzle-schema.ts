import {
	sqliteTable,
	text,
	real,
	integer,
	uniqueIndex,
	index
} from 'drizzle-orm/sqlite-core';

// ── clients ──────────────────────────────────────────────────────────
export const clients = sqliteTable(
	'clients',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').$defaultFn(() => crypto.randomUUID()),
		name: text('name').notNull(),
		email: text('email').default(''),
		phone: text('phone').default(''),
		address: text('address').default(''),
		pricing_tier_id: integer('pricing_tier_id').references(() => rateTiers.id, { onDelete: 'set null' }),
		metadata: text('metadata').default('{}'),
		payer_id: integer('payer_id').references(() => payers.id, { onDelete: 'set null' }),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
		updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [
		uniqueIndex('idx_clients_uuid').on(table.uuid),
		index('idx_clients_payer').on(table.payer_id)
	]
);

// ── invoices ─────────────────────────────────────────────────────────
export const invoices = sqliteTable(
	'invoices',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').$defaultFn(() => crypto.randomUUID()),
		invoice_number: text('invoice_number').notNull().unique(),
		client_id: integer('client_id')
			.notNull()
			.references(() => clients.id),
		date: text('date').notNull(),
		due_date: text('due_date').notNull(),
		payment_terms: text('payment_terms').default('custom'),
		subtotal: real('subtotal').default(0),
		tax_rate: real('tax_rate').default(0),
		tax_rate_id: integer('tax_rate_id').references(() => taxRates.id, { onDelete: 'set null' }),
		tax_amount: real('tax_amount').default(0),
		total: real('total').default(0),
		notes: text('notes').default(''),
		status: text('status').default('draft'),
		currency_code: text('currency_code').default('USD'),
		business_snapshot: text('business_snapshot').default('{}'),
		client_snapshot: text('client_snapshot').default('{}'),
		payer_snapshot: text('payer_snapshot').default('{}'),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
		updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [
		uniqueIndex('idx_invoices_uuid').on(table.uuid),
		index('idx_invoices_status').on(table.status),
		index('idx_invoices_client_id').on(table.client_id),
		index('idx_invoices_created_at').on(table.created_at)
	]
);

// ── line_items ───────────────────────────────────────────────────────
export const lineItems = sqliteTable(
	'line_items',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').$defaultFn(() => crypto.randomUUID()),
		invoice_id: integer('invoice_id')
			.notNull()
			.references(() => invoices.id, { onDelete: 'cascade' }),
		description: text('description').notNull(),
		quantity: real('quantity').notNull().default(1),
		rate: real('rate').notNull().default(0),
		amount: real('amount').notNull().default(0),
		notes: text('notes').default(''),
		sort_order: integer('sort_order').default(0),
		catalog_item_id: integer('catalog_item_id'),
		rate_tier_id: integer('rate_tier_id')
	},
	(table) => [index('idx_line_items_invoice_id').on(table.invoice_id)]
);

// ── catalog_items ────────────────────────────────────────────────────
export const catalogItems = sqliteTable(
	'catalog_items',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').$defaultFn(() => crypto.randomUUID()),
		name: text('name').notNull(),
		rate: real('rate').notNull().default(0),
		unit: text('unit').default(''),
		category: text('category').default(''),
		sku: text('sku').default(''),
		metadata: text('metadata').default('{}'),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
		updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [uniqueIndex('idx_catalog_items_uuid').on(table.uuid)]
);

// ── rate_tiers ───────────────────────────────────────────────────────
export const rateTiers = sqliteTable('rate_tiers', {
	id: integer('id').primaryKey({ autoIncrement: true }),
	uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
	name: text('name').notNull().unique(),
	description: text('description').default(''),
	sort_order: integer('sort_order').default(0),
	created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
	updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
});

// ── catalog_item_rates ───────────────────────────────────────────────
export const catalogItemRates = sqliteTable(
	'catalog_item_rates',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		catalog_item_id: integer('catalog_item_id')
			.notNull()
			.references(() => catalogItems.id, { onDelete: 'cascade' }),
		rate_tier_id: integer('rate_tier_id')
			.notNull()
			.references(() => rateTiers.id, { onDelete: 'cascade' }),
		rate: real('rate').notNull().default(0)
	},
	(table) => [
		uniqueIndex('idx_catalog_item_rates_unique').on(table.catalog_item_id, table.rate_tier_id)
	]
);

// ── column_mappings ──────────────────────────────────────────────────
export const columnMappings = sqliteTable('column_mappings', {
	id: integer('id').primaryKey({ autoIncrement: true }),
	uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
	name: text('name').notNull(),
	entity_type: text('entity_type').notNull().default('catalog'),
	mapping: text('mapping').notNull().default('{}'),
	tier_mapping: text('tier_mapping').default('{}'),
	metadata_mapping: text('metadata_mapping').default('[]'),
	file_type: text('file_type').default('csv'),
	sheet_name: text('sheet_name').default(''),
	header_row: integer('header_row').default(1),
	created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
	updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
});

// ── audit_log ────────────────────────────────────────────────────────
export const auditLog = sqliteTable(
	'audit_log',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
		entity_type: text('entity_type').notNull(),
		entity_id: integer('entity_id'),
		action: text('action').notNull(),
		changes: text('changes').default('{}'),
		context: text('context').default(''),
		batch_id: text('batch_id'),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [
		index('idx_audit_entity').on(table.entity_type, table.entity_id),
		index('idx_audit_batch').on(table.batch_id),
		index('idx_audit_created').on(table.created_at)
	]
);

// ── business_profile ─────────────────────────────────────────────────
export const businessProfile = sqliteTable(
	'business_profile',
	{
		id: integer('id').primaryKey(),
		uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
		name: text('name').notNull().default(''),
		email: text('email').default(''),
		phone: text('phone').default(''),
		address: text('address').default(''),
		logo: text('logo').default(''),
		metadata: text('metadata').default('{}'),
		default_currency: text('default_currency').default('USD'),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
		updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
	}
);

// ── payers ───────────────────────────────────────────────────────────
export const payers = sqliteTable('payers', {
	id: integer('id').primaryKey({ autoIncrement: true }),
	uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
	name: text('name').notNull(),
	email: text('email').default(''),
	phone: text('phone').default(''),
	address: text('address').default(''),
	metadata: text('metadata').default('{}'),
	created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
	updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
});

// ── estimates ────────────────────────────────────────────────────────
export const estimates = sqliteTable(
	'estimates',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').unique().$defaultFn(() => crypto.randomUUID()),
		estimate_number: text('estimate_number').unique().notNull(),
		client_id: integer('client_id').references(() => clients.id),
		date: text('date').notNull(),
		valid_until: text('valid_until').notNull(),
		subtotal: real('subtotal').default(0),
		tax_rate: real('tax_rate').default(0),
		tax_rate_id: integer('tax_rate_id').references(() => taxRates.id, { onDelete: 'set null' }),
		tax_amount: real('tax_amount').default(0),
		total: real('total').default(0),
		notes: text('notes').default(''),
		status: text('status').default('draft'),
		currency_code: text('currency_code').default('USD'),
		converted_invoice_id: integer('converted_invoice_id'),
		business_snapshot: text('business_snapshot').default('{}'),
		client_snapshot: text('client_snapshot').default('{}'),
		payer_snapshot: text('payer_snapshot').default('{}'),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
		updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [
		index('idx_estimates_status').on(table.status),
		index('idx_estimates_client_id').on(table.client_id)
	]
);

// ── estimate_line_items ──────────────────────────────────────────────
export const estimateLineItems = sqliteTable('estimate_line_items', {
	id: integer('id').primaryKey({ autoIncrement: true }),
	uuid: text('uuid').unique().$defaultFn(() => crypto.randomUUID()),
	estimate_id: integer('estimate_id').references(() => estimates.id, { onDelete: 'cascade' }),
	description: text('description').notNull(),
	quantity: real('quantity').default(1),
	rate: real('rate').default(0),
	amount: real('amount').default(0),
	notes: text('notes').default(''),
	sort_order: integer('sort_order').default(0),
	catalog_item_id: integer('catalog_item_id'),
	rate_tier_id: integer('rate_tier_id')
});

// ── tax_rates ────────────────────────────────────────────────────────
export const taxRates = sqliteTable('tax_rates', {
	id: integer('id').primaryKey({ autoIncrement: true }),
	uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
	name: text('name').notNull(),
	rate: real('rate').notNull().default(0),
	is_default: integer('is_default', { mode: 'boolean' }).notNull().default(false),
	created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
	updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
});

// ── payments ─────────────────────────────────────────────────────────
export const payments = sqliteTable(
	'payments',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
		invoice_id: integer('invoice_id')
			.notNull()
			.references(() => invoices.id, { onDelete: 'cascade' }),
		amount: real('amount').notNull(),
		payment_date: text('payment_date').notNull(),
		method: text('method').default(''),
		notes: text('notes').default(''),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
		updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [index('idx_payments_invoice_id').on(table.invoice_id)]
);

// ── ai chat ──────────────────────────────────────────────────────────
export const aiChatSessions = sqliteTable('ai_chat_sessions', {
	id: integer('id').primaryKey({ autoIncrement: true }),
	uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
	title: text('title').notNull().default('New chat'),
	loaded_skills_json: text('loaded_skills_json').notNull().default('[]'),
	created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
	updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
});

export const aiChatMessages = sqliteTable(
	'ai_chat_messages',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		session_id: integer('session_id')
			.notNull()
			.references(() => aiChatSessions.id, { onDelete: 'cascade' }),
		role: text('role').notNull(),
		content: text('content').notNull().default(''),
		tool_calls: text('tool_calls').notNull().default('[]'),
		tool_call_id: text('tool_call_id').default(''),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [index('idx_ai_chat_messages_session').on(table.session_id)]
);

export const aiChatToolCalls = sqliteTable(
	'ai_chat_tool_calls',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
		session_id: integer('session_id')
			.notNull()
			.references(() => aiChatSessions.id, { onDelete: 'cascade' }),
		message_id: integer('message_id').references(() => aiChatMessages.id, { onDelete: 'cascade' }),
		tool_name: text('tool_name').notNull(),
		args_json: text('args_json').notNull().default('{}'),
		status: text('status').notNull().default('pending'),
		result_json: text('result_json').notNull().default('null'),
		error_message: text('error_message').default(''),
		parent_tool_call_uuid: text('parent_tool_call_uuid'),
		agent_id: text('agent_id'),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
		updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [index('idx_ai_chat_tool_calls_session').on(table.session_id)]
);

// ── recurring_templates ──────────────────────────────────────────────
export const recurringTemplates = sqliteTable(
	'recurring_templates',
	{
		id: integer('id').primaryKey({ autoIncrement: true }),
		uuid: text('uuid').notNull().unique().$defaultFn(() => crypto.randomUUID()),
		client_id: integer('client_id').references(() => clients.id, { onDelete: 'set null' }),
		name: text('name').notNull(),
		frequency: text('frequency').notNull(),
		next_due: text('next_due').notNull(),
		line_items: text('line_items').notNull().default('[]'),
		tax_rate: real('tax_rate').notNull().default(0),
		notes: text('notes').notNull().default(''),
		is_active: integer('is_active', { mode: 'boolean' }).notNull().default(true),
		created_at: text('created_at').$defaultFn(() => new Date().toISOString()),
		updated_at: text('updated_at').$defaultFn(() => new Date().toISOString())
	},
	(table) => [
		index('idx_recurring_client').on(table.client_id),
		index('idx_recurring_next_due').on(table.next_due)
	]
);

