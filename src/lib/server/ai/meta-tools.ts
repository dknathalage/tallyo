import type { ToolSpec } from './tools.js';
import { listSkills, getSkill } from './skills.js';

export const META_TOOL_NAMES = ['loadSkill', 'listAvailableSkills'] as const;

export interface MetaToolContext {
	sessionId: number;
	loadedSkills: Set<string>;
	persistLoadedSkills(ids: string[]): Promise<void>;
	onSkillLoaded(skillId: string): void;
	stopController: AbortController;
}

export function makeMetaTools(ctx: MetaToolContext): ToolSpec[] {
	return [
		{
			name: 'loadSkill',
			description:
				'Enable a skill (a bundle of tools for one domain) so its tools become available next turn. Use this when the user asks about a domain whose tools are not currently loaded. After calling, stop and wait for the continuation.',
			kind: 'read',
			paramSchema: {
				type: 'object',
				properties: { id: { type: 'string' } },
				required: ['id']
			},
			execute: async (args) => {
				const id = typeof args['id'] === 'string' ? args['id'] : '';
				const skill = getSkill(id);
				if (!skill) return { error: `unknown skill: ${id}` };
				if (skill.alwaysLoaded) return { loaded: id, already: true };
				ctx.loadedSkills.add(id);
				await ctx.persistLoadedSkills(Array.from(ctx.loadedSkills));
				ctx.onSkillLoaded(id);
				queueMicrotask(() => ctx.stopController.abort());
				return {
					loaded: id,
					tools_added: skill.tools,
					message: 'Skill loaded. Generation stopping; will continue with new tools.'
				};
			}
		},
		{
			name: 'listAvailableSkills',
			description:
				'List all skills (loaded and not loaded). Useful when you are not sure which skill to load.',
			kind: 'read',
			paramSchema: { type: 'object', properties: {} },
			execute: async () =>
				listSkills().map((s) => ({
					id: s.id,
					title: s.title,
					description: s.description,
					tool_count: s.tools.length,
					loaded: ctx.loadedSkills.has(s.id) || s.alwaysLoaded === true
				}))
		}
	];
}
