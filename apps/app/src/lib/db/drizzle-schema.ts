import {
	pgTable,
	serial,
	text,
	doublePrecision,
	integer,
	boolean,
	timestamp,
	uuid,
	uniqueIndex,
	index,
	check
} from 'drizzle-orm/pg-core';
import { sql } from 'drizzle-orm';

// ── clients ──────────────────────────────────────────────────────────
export const clients = pgTable(
	'clients',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').defaultRandom(),
		name: text('name').notNull(),
		email: text('email').default(''),
		phone: text('phone').default(''),
		address: text('address').default(''),
		pricing_tier_id: integer('pricing_tier_id').references(() => rateTiers.id, { onDelete: 'set null' }),
		metadata: text('metadata').default('{}'),
		payer_id: integer('payer_id').references(() => payers.id, { onDelete: 'set null' }),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
		updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
	},
	(table) => [
		uniqueIndex('idx_clients_uuid').on(table.uuid),
		index('idx_clients_payer').on(table.payer_id)
	]
);

// ── invoices ─────────────────────────────────────────────────────────
export const invoices = pgTable(
	'invoices',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').defaultRandom(),
		invoice_number: text('invoice_number').notNull().unique(),
		client_id: integer('client_id')
			.notNull()
			.references(() => clients.id),
		date: text('date').notNull(),
		due_date: text('due_date').notNull(),
		payment_terms: text('payment_terms').default('custom'),
		subtotal: doublePrecision('subtotal').default(0),
		tax_rate: doublePrecision('tax_rate').default(0),
		tax_rate_id: integer('tax_rate_id').references(() => taxRates.id, { onDelete: 'set null' }),
		tax_amount: doublePrecision('tax_amount').default(0),
		total: doublePrecision('total').default(0),
		notes: text('notes').default(''),
		status: text('status').default('draft'),
		currency_code: text('currency_code').default('USD'),
		business_snapshot: text('business_snapshot').default('{}'),
		client_snapshot: text('client_snapshot').default('{}'),
		payer_snapshot: text('payer_snapshot').default('{}'),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
		updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
	},
	(table) => [
		uniqueIndex('idx_invoices_uuid').on(table.uuid),
		index('idx_invoices_status').on(table.status),
		index('idx_invoices_client_id').on(table.client_id),
		index('idx_invoices_created_at').on(table.created_at)
	]
);

// ── line_items ───────────────────────────────────────────────────────
export const lineItems = pgTable(
	'line_items',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').defaultRandom(),
		invoice_id: integer('invoice_id')
			.notNull()
			.references(() => invoices.id, { onDelete: 'cascade' }),
		description: text('description').notNull(),
		quantity: doublePrecision('quantity').notNull().default(1),
		rate: doublePrecision('rate').notNull().default(0),
		amount: doublePrecision('amount').notNull().default(0),
		notes: text('notes').default(''),
		sort_order: integer('sort_order').default(0),
		catalog_item_id: integer('catalog_item_id'),
		rate_tier_id: integer('rate_tier_id')
	},
	(table) => [index('idx_line_items_invoice_id').on(table.invoice_id)]
);

// ── catalog_items ────────────────────────────────────────────────────
export const catalogItems = pgTable(
	'catalog_items',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').defaultRandom(),
		name: text('name').notNull(),
		rate: doublePrecision('rate').notNull().default(0),
		unit: text('unit').default(''),
		category: text('category').default(''),
		sku: text('sku').default(''),
		metadata: text('metadata').default('{}'),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
		updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
	},
	(table) => [uniqueIndex('idx_catalog_items_uuid').on(table.uuid)]
);

// ── rate_tiers ───────────────────────────────────────────────────────
export const rateTiers = pgTable('rate_tiers', {
	id: serial('id').primaryKey(),
	uuid: uuid('uuid').notNull().unique().defaultRandom(),
	name: text('name').notNull().unique(),
	description: text('description').default(''),
	sort_order: integer('sort_order').default(0),
	created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
	updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
});

// ── catalog_item_rates ───────────────────────────────────────────────
export const catalogItemRates = pgTable(
	'catalog_item_rates',
	{
		id: serial('id').primaryKey(),
		catalog_item_id: integer('catalog_item_id')
			.notNull()
			.references(() => catalogItems.id, { onDelete: 'cascade' }),
		rate_tier_id: integer('rate_tier_id')
			.notNull()
			.references(() => rateTiers.id, { onDelete: 'cascade' }),
		rate: doublePrecision('rate').notNull().default(0)
	},
	(table) => [
		uniqueIndex('idx_catalog_item_rates_unique').on(table.catalog_item_id, table.rate_tier_id)
	]
);

// ── column_mappings ──────────────────────────────────────────────────
export const columnMappings = pgTable('column_mappings', {
	id: serial('id').primaryKey(),
	uuid: uuid('uuid').notNull().unique().defaultRandom(),
	name: text('name').notNull(),
	entity_type: text('entity_type').notNull().default('catalog'),
	mapping: text('mapping').notNull().default('{}'),
	tier_mapping: text('tier_mapping').default('{}'),
	metadata_mapping: text('metadata_mapping').default('[]'),
	file_type: text('file_type').default('csv'),
	sheet_name: text('sheet_name').default(''),
	header_row: integer('header_row').default(1),
	created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
	updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
});

// ── audit_log ────────────────────────────────────────────────────────
export const auditLog = pgTable(
	'audit_log',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').notNull().unique().defaultRandom(),
		entity_type: text('entity_type').notNull(),
		entity_id: integer('entity_id'),
		action: text('action').notNull(),
		changes: text('changes').default('{}'),
		context: text('context').default(''),
		batch_id: text('batch_id'),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow()
	},
	(table) => [
		index('idx_audit_entity').on(table.entity_type, table.entity_id),
		index('idx_audit_batch').on(table.batch_id),
		index('idx_audit_created').on(table.created_at)
	]
);

// ── business_profile ─────────────────────────────────────────────────
export const businessProfile = pgTable(
	'business_profile',
	{
		id: integer('id').primaryKey(),
		uuid: uuid('uuid').notNull().unique().defaultRandom(),
		name: text('name').notNull().default(''),
		email: text('email').default(''),
		phone: text('phone').default(''),
		address: text('address').default(''),
		logo: text('logo').default(''),
		metadata: text('metadata').default('{}'),
		default_currency: text('default_currency').default('USD'),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
		updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
	},
	(table) => [check('business_profile_single_row', sql`${table.id} = 1`)]
);

// ── payers ───────────────────────────────────────────────────────────
export const payers = pgTable('payers', {
	id: serial('id').primaryKey(),
	uuid: uuid('uuid').notNull().unique().defaultRandom(),
	name: text('name').notNull(),
	email: text('email').default(''),
	phone: text('phone').default(''),
	address: text('address').default(''),
	metadata: text('metadata').default('{}'),
	created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
	updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
});

// ── estimates ────────────────────────────────────────────────────────
export const estimates = pgTable(
	'estimates',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').unique().defaultRandom(),
		estimate_number: text('estimate_number').unique().notNull(),
		client_id: integer('client_id').references(() => clients.id),
		date: text('date').notNull(),
		valid_until: text('valid_until').notNull(),
		subtotal: doublePrecision('subtotal').default(0),
		tax_rate: doublePrecision('tax_rate').default(0),
		tax_rate_id: integer('tax_rate_id').references(() => taxRates.id, { onDelete: 'set null' }),
		tax_amount: doublePrecision('tax_amount').default(0),
		total: doublePrecision('total').default(0),
		notes: text('notes').default(''),
		status: text('status').default('draft'),
		currency_code: text('currency_code').default('USD'),
		converted_invoice_id: integer('converted_invoice_id'),
		business_snapshot: text('business_snapshot').default('{}'),
		client_snapshot: text('client_snapshot').default('{}'),
		payer_snapshot: text('payer_snapshot').default('{}'),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
		updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
	},
	(table) => [
		index('idx_estimates_status').on(table.status),
		index('idx_estimates_client_id').on(table.client_id)
	]
);

// ── estimate_line_items ──────────────────────────────────────────────
export const estimateLineItems = pgTable('estimate_line_items', {
	id: serial('id').primaryKey(),
	uuid: uuid('uuid').unique().defaultRandom(),
	estimate_id: integer('estimate_id').references(() => estimates.id, { onDelete: 'cascade' }),
	description: text('description').notNull(),
	quantity: doublePrecision('quantity').default(1),
	rate: doublePrecision('rate').default(0),
	amount: doublePrecision('amount').default(0),
	notes: text('notes').default(''),
	sort_order: integer('sort_order').default(0),
	catalog_item_id: integer('catalog_item_id'),
	rate_tier_id: integer('rate_tier_id')
});

// ── tax_rates ────────────────────────────────────────────────────────
export const taxRates = pgTable('tax_rates', {
	id: serial('id').primaryKey(),
	uuid: uuid('uuid').notNull().unique().defaultRandom(),
	name: text('name').notNull(),
	rate: doublePrecision('rate').notNull().default(0),
	is_default: boolean('is_default').notNull().default(false),
	created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
	updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
});

// ── payments ─────────────────────────────────────────────────────────
export const payments = pgTable(
	'payments',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').notNull().unique().defaultRandom(),
		invoice_id: integer('invoice_id')
			.notNull()
			.references(() => invoices.id, { onDelete: 'cascade' }),
		amount: doublePrecision('amount').notNull(),
		payment_date: text('payment_date').notNull(),
		method: text('method').default(''),
		notes: text('notes').default(''),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
		updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
	},
	(table) => [index('idx_payments_invoice_id').on(table.invoice_id)]
);

// ── recurring_templates ──────────────────────────────────────────────
export const recurringTemplates = pgTable(
	'recurring_templates',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').notNull().unique().defaultRandom(),
		client_id: integer('client_id').references(() => clients.id, { onDelete: 'set null' }),
		name: text('name').notNull(),
		frequency: text('frequency').notNull(),
		next_due: text('next_due').notNull(),
		line_items: text('line_items').notNull().default('[]'),
		tax_rate: doublePrecision('tax_rate').notNull().default(0),
		notes: text('notes').notNull().default(''),
		is_active: boolean('is_active').notNull().default(true),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
		updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
	},
	(table) => [
		index('idx_recurring_client').on(table.client_id),
		index('idx_recurring_next_due').on(table.next_due)
	]
);

// ── ai_chat_sessions ────────────────────────────────────────────────
export const aiChatSessions = pgTable(
	'ai_chat_sessions',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').notNull().unique().defaultRandom(),
		title: text('title').notNull().default('New Chat'),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow(),
		updated_at: timestamp('updated_at', { withTimezone: true }).defaultNow()
	},
	(table) => [index('idx_ai_sessions_created').on(table.created_at)]
);

// ── ai_chat_messages ────────────────────────────────────────────────
export const aiChatMessages = pgTable(
	'ai_chat_messages',
	{
		id: serial('id').primaryKey(),
		uuid: uuid('uuid').notNull().unique().defaultRandom(),
		session_id: integer('session_id')
			.notNull()
			.references(() => aiChatSessions.id, { onDelete: 'cascade' }),
		role: text('role').notNull(),
		content: text('content').notNull().default(''),
		tool_calls: text('tool_calls'),
		tool_results: text('tool_results'),
		is_streaming: boolean('is_streaming').notNull().default(false),
		created_at: timestamp('created_at', { withTimezone: true }).defaultNow()
	},
	(table) => [index('idx_ai_messages_session').on(table.session_id, table.created_at)]
);
