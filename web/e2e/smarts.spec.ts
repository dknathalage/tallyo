import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { createClient } from './fixtures';

// Live Smarts UI test. Gated: only runs with SMARTS_E2E=1, which also makes
// launch.sh load .env (ANTHROPIC_API_KEY) so the server enables Smarts. The
// default suite skips this and makes no paid API calls.
const RUN = process.env.SMARTS_E2E === '1';
const { tenant } = JSON.parse(readFileSync('e2e/.auth/tenant.json', 'utf8')) as { tenant: string };

test.describe('smarts (live)', () => {
	test.skip(!RUN, 'set SMARTS_E2E=1 (needs ANTHROPIC_API_KEY in .env) to run');

	test('draft follow-up reminder on a sent invoice', async ({ page }) => {
		const clientId = await createClient(page.request, tenant, 'Reminder Co');
		const res = await page.request.post(`/api/t/${tenant}/invoices`, {
			data: {
				clientId,
				status: 'sent',
				issueDate: '2026-05-01',
				dueDate: '2026-05-15',
				lineItems: [{ description: 'Consulting', quantity: 1, unitPrice: 150, taxable: true }]
			}
		});
		expect(res.ok(), `seed invoice failed: ${res.status()} ${await res.text()}`).toBeTruthy();
		const inv = await res.json();

		await page.goto(`/${tenant}/invoices/${inv.id}`);
		// Button only renders once the features store reports smarts enabled.
		await page.getByRole('button', { name: 'Draft reminder' }).click();

		// The model fills Subject + Body; assert the drafted body shows up non-empty.
		await expect(page.getByRole('textbox', { name: 'Body' })).toHaveValue(/\S/, { timeout: 30_000 });
		await expect(page.getByRole('textbox', { name: 'Subject' })).toHaveValue(/\S/);
	});
});
