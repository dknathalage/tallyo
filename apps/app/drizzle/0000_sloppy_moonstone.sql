CREATE TABLE `ai_chat_messages` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`session_id` integer NOT NULL,
	`role` text NOT NULL,
	`content` text DEFAULT '' NOT NULL,
	`tool_calls` text,
	`tool_results` text,
	`is_streaming` integer DEFAULT false NOT NULL,
	`created_at` text,
	FOREIGN KEY (`session_id`) REFERENCES `ai_chat_sessions`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `ai_chat_messages_uuid_unique` ON `ai_chat_messages` (`uuid`);--> statement-breakpoint
CREATE INDEX `idx_ai_messages_session` ON `ai_chat_messages` (`session_id`,`created_at`);--> statement-breakpoint
CREATE TABLE `ai_chat_sessions` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`title` text DEFAULT 'New Chat' NOT NULL,
	`created_at` text,
	`updated_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `ai_chat_sessions_uuid_unique` ON `ai_chat_sessions` (`uuid`);--> statement-breakpoint
CREATE INDEX `idx_ai_sessions_created` ON `ai_chat_sessions` (`created_at`);--> statement-breakpoint
CREATE TABLE `audit_log` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`entity_type` text NOT NULL,
	`entity_id` integer,
	`action` text NOT NULL,
	`changes` text DEFAULT '{}',
	`context` text DEFAULT '',
	`batch_id` text,
	`created_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `audit_log_uuid_unique` ON `audit_log` (`uuid`);--> statement-breakpoint
CREATE INDEX `idx_audit_entity` ON `audit_log` (`entity_type`,`entity_id`);--> statement-breakpoint
CREATE INDEX `idx_audit_batch` ON `audit_log` (`batch_id`);--> statement-breakpoint
CREATE INDEX `idx_audit_created` ON `audit_log` (`created_at`);--> statement-breakpoint
CREATE TABLE `business_profile` (
	`id` integer PRIMARY KEY NOT NULL,
	`uuid` text NOT NULL,
	`name` text DEFAULT '' NOT NULL,
	`email` text DEFAULT '',
	`phone` text DEFAULT '',
	`address` text DEFAULT '',
	`logo` text DEFAULT '',
	`metadata` text DEFAULT '{}',
	`default_currency` text DEFAULT 'USD',
	`created_at` text,
	`updated_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `business_profile_uuid_unique` ON `business_profile` (`uuid`);--> statement-breakpoint
CREATE TABLE `catalog_item_rates` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`catalog_item_id` integer NOT NULL,
	`rate_tier_id` integer NOT NULL,
	`rate` real DEFAULT 0 NOT NULL,
	FOREIGN KEY (`catalog_item_id`) REFERENCES `catalog_items`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`rate_tier_id`) REFERENCES `rate_tiers`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `idx_catalog_item_rates_unique` ON `catalog_item_rates` (`catalog_item_id`,`rate_tier_id`);--> statement-breakpoint
CREATE TABLE `catalog_items` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text,
	`name` text NOT NULL,
	`rate` real DEFAULT 0 NOT NULL,
	`unit` text DEFAULT '',
	`category` text DEFAULT '',
	`sku` text DEFAULT '',
	`metadata` text DEFAULT '{}',
	`created_at` text,
	`updated_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `idx_catalog_items_uuid` ON `catalog_items` (`uuid`);--> statement-breakpoint
CREATE TABLE `clients` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text,
	`name` text NOT NULL,
	`email` text DEFAULT '',
	`phone` text DEFAULT '',
	`address` text DEFAULT '',
	`pricing_tier_id` integer,
	`metadata` text DEFAULT '{}',
	`payer_id` integer,
	`created_at` text,
	`updated_at` text,
	FOREIGN KEY (`pricing_tier_id`) REFERENCES `rate_tiers`(`id`) ON UPDATE no action ON DELETE set null,
	FOREIGN KEY (`payer_id`) REFERENCES `payers`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE UNIQUE INDEX `idx_clients_uuid` ON `clients` (`uuid`);--> statement-breakpoint
CREATE INDEX `idx_clients_payer` ON `clients` (`payer_id`);--> statement-breakpoint
CREATE TABLE `column_mappings` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`name` text NOT NULL,
	`entity_type` text DEFAULT 'catalog' NOT NULL,
	`mapping` text DEFAULT '{}' NOT NULL,
	`tier_mapping` text DEFAULT '{}',
	`metadata_mapping` text DEFAULT '[]',
	`file_type` text DEFAULT 'csv',
	`sheet_name` text DEFAULT '',
	`header_row` integer DEFAULT 1,
	`created_at` text,
	`updated_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `column_mappings_uuid_unique` ON `column_mappings` (`uuid`);--> statement-breakpoint
CREATE TABLE `estimate_line_items` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text,
	`estimate_id` integer,
	`description` text NOT NULL,
	`quantity` real DEFAULT 1,
	`rate` real DEFAULT 0,
	`amount` real DEFAULT 0,
	`notes` text DEFAULT '',
	`sort_order` integer DEFAULT 0,
	`catalog_item_id` integer,
	`rate_tier_id` integer,
	FOREIGN KEY (`estimate_id`) REFERENCES `estimates`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `estimate_line_items_uuid_unique` ON `estimate_line_items` (`uuid`);--> statement-breakpoint
CREATE TABLE `estimates` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text,
	`estimate_number` text NOT NULL,
	`client_id` integer,
	`date` text NOT NULL,
	`valid_until` text NOT NULL,
	`subtotal` real DEFAULT 0,
	`tax_rate` real DEFAULT 0,
	`tax_rate_id` integer,
	`tax_amount` real DEFAULT 0,
	`total` real DEFAULT 0,
	`notes` text DEFAULT '',
	`status` text DEFAULT 'draft',
	`currency_code` text DEFAULT 'USD',
	`converted_invoice_id` integer,
	`business_snapshot` text DEFAULT '{}',
	`client_snapshot` text DEFAULT '{}',
	`payer_snapshot` text DEFAULT '{}',
	`created_at` text,
	`updated_at` text,
	FOREIGN KEY (`client_id`) REFERENCES `clients`(`id`) ON UPDATE no action ON DELETE no action,
	FOREIGN KEY (`tax_rate_id`) REFERENCES `tax_rates`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE UNIQUE INDEX `estimates_uuid_unique` ON `estimates` (`uuid`);--> statement-breakpoint
CREATE UNIQUE INDEX `estimates_estimate_number_unique` ON `estimates` (`estimate_number`);--> statement-breakpoint
CREATE INDEX `idx_estimates_status` ON `estimates` (`status`);--> statement-breakpoint
CREATE INDEX `idx_estimates_client_id` ON `estimates` (`client_id`);--> statement-breakpoint
CREATE TABLE `invoices` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text,
	`invoice_number` text NOT NULL,
	`client_id` integer NOT NULL,
	`date` text NOT NULL,
	`due_date` text NOT NULL,
	`payment_terms` text DEFAULT 'custom',
	`subtotal` real DEFAULT 0,
	`tax_rate` real DEFAULT 0,
	`tax_rate_id` integer,
	`tax_amount` real DEFAULT 0,
	`total` real DEFAULT 0,
	`notes` text DEFAULT '',
	`status` text DEFAULT 'draft',
	`currency_code` text DEFAULT 'USD',
	`business_snapshot` text DEFAULT '{}',
	`client_snapshot` text DEFAULT '{}',
	`payer_snapshot` text DEFAULT '{}',
	`created_at` text,
	`updated_at` text,
	FOREIGN KEY (`client_id`) REFERENCES `clients`(`id`) ON UPDATE no action ON DELETE no action,
	FOREIGN KEY (`tax_rate_id`) REFERENCES `tax_rates`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE UNIQUE INDEX `invoices_invoice_number_unique` ON `invoices` (`invoice_number`);--> statement-breakpoint
CREATE UNIQUE INDEX `idx_invoices_uuid` ON `invoices` (`uuid`);--> statement-breakpoint
CREATE INDEX `idx_invoices_status` ON `invoices` (`status`);--> statement-breakpoint
CREATE INDEX `idx_invoices_client_id` ON `invoices` (`client_id`);--> statement-breakpoint
CREATE INDEX `idx_invoices_created_at` ON `invoices` (`created_at`);--> statement-breakpoint
CREATE TABLE `line_items` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text,
	`invoice_id` integer NOT NULL,
	`description` text NOT NULL,
	`quantity` real DEFAULT 1 NOT NULL,
	`rate` real DEFAULT 0 NOT NULL,
	`amount` real DEFAULT 0 NOT NULL,
	`notes` text DEFAULT '',
	`sort_order` integer DEFAULT 0,
	`catalog_item_id` integer,
	`rate_tier_id` integer,
	FOREIGN KEY (`invoice_id`) REFERENCES `invoices`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `idx_line_items_invoice_id` ON `line_items` (`invoice_id`);--> statement-breakpoint
CREATE TABLE `payers` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`name` text NOT NULL,
	`email` text DEFAULT '',
	`phone` text DEFAULT '',
	`address` text DEFAULT '',
	`metadata` text DEFAULT '{}',
	`created_at` text,
	`updated_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `payers_uuid_unique` ON `payers` (`uuid`);--> statement-breakpoint
CREATE TABLE `payments` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`invoice_id` integer NOT NULL,
	`amount` real NOT NULL,
	`payment_date` text NOT NULL,
	`method` text DEFAULT '',
	`notes` text DEFAULT '',
	`created_at` text,
	`updated_at` text,
	FOREIGN KEY (`invoice_id`) REFERENCES `invoices`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `payments_uuid_unique` ON `payments` (`uuid`);--> statement-breakpoint
CREATE INDEX `idx_payments_invoice_id` ON `payments` (`invoice_id`);--> statement-breakpoint
CREATE TABLE `rate_tiers` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`name` text NOT NULL,
	`description` text DEFAULT '',
	`sort_order` integer DEFAULT 0,
	`created_at` text,
	`updated_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `rate_tiers_uuid_unique` ON `rate_tiers` (`uuid`);--> statement-breakpoint
CREATE UNIQUE INDEX `rate_tiers_name_unique` ON `rate_tiers` (`name`);--> statement-breakpoint
CREATE TABLE `recurring_templates` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`client_id` integer,
	`name` text NOT NULL,
	`frequency` text NOT NULL,
	`next_due` text NOT NULL,
	`line_items` text DEFAULT '[]' NOT NULL,
	`tax_rate` real DEFAULT 0 NOT NULL,
	`notes` text DEFAULT '' NOT NULL,
	`is_active` integer DEFAULT true NOT NULL,
	`created_at` text,
	`updated_at` text,
	FOREIGN KEY (`client_id`) REFERENCES `clients`(`id`) ON UPDATE no action ON DELETE set null
);
--> statement-breakpoint
CREATE UNIQUE INDEX `recurring_templates_uuid_unique` ON `recurring_templates` (`uuid`);--> statement-breakpoint
CREATE INDEX `idx_recurring_client` ON `recurring_templates` (`client_id`);--> statement-breakpoint
CREATE INDEX `idx_recurring_next_due` ON `recurring_templates` (`next_due`);--> statement-breakpoint
CREATE TABLE `tax_rates` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`name` text NOT NULL,
	`rate` real DEFAULT 0 NOT NULL,
	`is_default` integer DEFAULT false NOT NULL,
	`created_at` text,
	`updated_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `tax_rates_uuid_unique` ON `tax_rates` (`uuid`);