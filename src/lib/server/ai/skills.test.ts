import { describe, it, expect } from 'vitest';
import { listSkills, getSkill, resolveSkillsForContext } from './skills.js';
import { listTools } from './tools.js';
import { META_TOOL_NAMES } from './meta-tools.js';
import { AGENT_TOOL_NAMES } from './agents.js';

describe('skills registry', () => {
	it('every tool name in every skill resolves via getTool, meta-tools, or agent tools', () => {
		const builtin = new Set(listTools().map((t) => t.name));
		const meta = new Set<string>(META_TOOL_NAMES);
		const agents = new Set<string>(AGENT_TOOL_NAMES);
		const missing: string[] = [];
		for (const skill of listSkills()) {
			for (const name of skill.tools) {
				if (builtin.has(name) || meta.has(name) || agents.has(name)) continue;
				missing.push(`${skill.id}::${name}`);
			}
		}
		expect(missing).toEqual([]);
	});

	it('exposes core as alwaysLoaded', () => {
		const core = getSkill('core');
		expect(core?.alwaysLoaded).toBe(true);
	});
});

describe('resolveSkillsForContext', () => {
	it('always includes core', () => {
		const result = resolveSkillsForContext({});
		expect(result.some((s) => s.id === 'core')).toBe(true);
	});

	it('matches skills by route', () => {
		const result = resolveSkillsForContext({ route: '/console/invoices' });
		const ids = result.map((s) => s.id);
		expect(ids).toContain('invoices');
		expect(ids).toContain('core');
	});

	it('payments skill matches invoice detail route only', () => {
		const detail = resolveSkillsForContext({ route: '/console/invoices/42' }).map((s) => s.id);
		expect(detail).toContain('payments');
		expect(detail).toContain('invoices');
		const list = resolveSkillsForContext({ route: '/console/invoices' }).map((s) => s.id);
		expect(list).not.toContain('payments');
	});

	it('matches skills by keyword in user message', () => {
		const result = resolveSkillsForContext({ userMessage: 'show me the catalog' });
		expect(result.map((s) => s.id)).toContain('catalog');
	});

	it('explicitly loaded skills are included', () => {
		const result = resolveSkillsForContext({ explicitlyLoaded: ['recurring'] });
		expect(result.map((s) => s.id)).toContain('recurring');
	});

	it('caps non-core skills to opts.cap (default 4)', () => {
		const result = resolveSkillsForContext({
			explicitlyLoaded: ['clients', 'invoices', 'estimates', 'catalog', 'payers']
		});
		const nonCore = result.filter((s) => !s.alwaysLoaded);
		expect(nonCore.length).toBeLessThanOrEqual(4);
	});

	it('dedups: explicit + route same id appears once', () => {
		const result = resolveSkillsForContext({
			route: '/console/invoices',
			explicitlyLoaded: ['invoices']
		});
		const invCount = result.filter((s) => s.id === 'invoices').length;
		expect(invCount).toBe(1);
	});

	it('explicit takes priority over route under cap', () => {
		// cap 1 with one explicit — route skill should be dropped
		const result = resolveSkillsForContext({
			route: '/console/invoices',
			explicitlyLoaded: ['recurring'],
			cap: 1
		});
		const ids = result.map((s) => s.id);
		expect(ids).toContain('recurring');
		expect(ids).not.toContain('invoices');
	});
});
