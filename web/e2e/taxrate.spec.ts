import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { createClient } from './fixtures';

// Tax-rate e2e: create a default tax rate via the API, seed a taxable invoice,
// navigate to its detail, and assert the tax amount renders in the total section.
const { tenant } = JSON.parse(readFileSync('e2e/.auth/tenant.json', 'utf8')) as { tenant: string };

test('tax rates list page shows heading and New button', async ({ page }) => {
	await page.goto(`/${tenant}/tax-rates`);
	await expect(page.getByRole('heading', { name: 'Tax rates' })).toBeVisible();
	// "New" button is rendered by DataTable for the tax-rates table.
	await expect(page.getByRole('button', { name: 'New' })).toBeVisible();
});

test('default tax rate applied: taxable line item shows tax in invoice total', async ({ page }) => {
	// Create a default tax rate via the API.
	// The rate is stored as a raw decimal multiplier: 0.1 = 10%, 0.2 = 20%.
	// Billing computes tax = Round2(lineTotal * rate), so 0.1 on $100 = $10.
	const rateRes = await page.request.post(`/api/t/${tenant}/tax-rates`, {
		data: { name: 'GST 10%', rate: 0.1, isDefault: true }
	});
	expect(rateRes.ok(), `seed tax rate failed: ${rateRes.status()} ${await rateRes.text()}`).toBeTruthy();

	// Seed a client and an invoice with a taxable line item.
	const clientId = await createClient(page.request, tenant, 'Tax Test Client');
	const today = new Date().toISOString().slice(0, 10);
	const invRes = await page.request.post(`/api/t/${tenant}/invoices`, {
		data: {
			clientId,
			payerId: null,
			status: 'draft',
			issueDate: today,
			dueDate: today,
			notes: '',
			lineItems: [
				{
					// No code + no customItemId → not a catalogue line, so no
					// price-list version is required. taxable: true causes the server
					// to apply the default 10% rate → tax = 10, total = 110.
					description: 'Taxable consulting',
					quantity: 1,
					unitPrice: 100,
					taxable: true
				}
			]
		}
	});
	expect(invRes.ok(), `seed invoice failed: ${invRes.status()} ${await invRes.text()}`).toBeTruthy();
	const inv = await invRes.json();

	// Navigate to the invoice detail page.
	await page.goto(`/${tenant}/invoices/${inv.id}`);

	// Wait for the line-items table to render, then assert the total reflects tax:
	// 100 (line) + 10 (10% of 100) = 110.
	await expect(page.getByText('Total').first()).toBeVisible();
	await expect(page.getByText('$110.00').first()).toBeVisible();
});
