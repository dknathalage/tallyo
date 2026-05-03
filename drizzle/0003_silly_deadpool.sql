ALTER TABLE `ai_chat_sessions` ADD `loaded_skills_json` text DEFAULT '[]' NOT NULL;--> statement-breakpoint
ALTER TABLE `ai_chat_tool_calls` ADD `parent_tool_call_uuid` text;--> statement-breakpoint
ALTER TABLE `ai_chat_tool_calls` ADD `agent_id` text;