CREATE TABLE `ai_chat_messages` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`session_id` integer NOT NULL,
	`role` text NOT NULL,
	`content` text DEFAULT '' NOT NULL,
	`tool_calls` text DEFAULT '[]' NOT NULL,
	`tool_call_id` text DEFAULT '',
	`created_at` text,
	FOREIGN KEY (`session_id`) REFERENCES `ai_chat_sessions`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE INDEX `idx_ai_chat_messages_session` ON `ai_chat_messages` (`session_id`);--> statement-breakpoint
CREATE TABLE `ai_chat_sessions` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`title` text DEFAULT 'New chat' NOT NULL,
	`created_at` text,
	`updated_at` text
);
--> statement-breakpoint
CREATE UNIQUE INDEX `ai_chat_sessions_uuid_unique` ON `ai_chat_sessions` (`uuid`);--> statement-breakpoint
CREATE TABLE `ai_chat_tool_calls` (
	`id` integer PRIMARY KEY AUTOINCREMENT NOT NULL,
	`uuid` text NOT NULL,
	`session_id` integer NOT NULL,
	`message_id` integer,
	`tool_name` text NOT NULL,
	`args_json` text DEFAULT '{}' NOT NULL,
	`status` text DEFAULT 'pending' NOT NULL,
	`result_json` text DEFAULT 'null' NOT NULL,
	`error_message` text DEFAULT '',
	`created_at` text,
	`updated_at` text,
	FOREIGN KEY (`session_id`) REFERENCES `ai_chat_sessions`(`id`) ON UPDATE no action ON DELETE cascade,
	FOREIGN KEY (`message_id`) REFERENCES `ai_chat_messages`(`id`) ON UPDATE no action ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `ai_chat_tool_calls_uuid_unique` ON `ai_chat_tool_calls` (`uuid`);--> statement-breakpoint
CREATE INDEX `idx_ai_chat_tool_calls_session` ON `ai_chat_tool_calls` (`session_id`);