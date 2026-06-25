import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { createClient } from './fixtures';

// Invoice lifecycle e2e: create via API, view detail, assert total renders,
// advance status draft → sent → paid, assert the paid badge shows.
const { tenant } = JSON.parse(readFileSync('e2e/.auth/tenant.json', 'utf8')) as { tenant: string };

test('invoice create → line item total renders → mark sent → mark paid', async ({ page }) => {
	// Seed a client and invoice via the API so the test drives only the lifecycle
	// UI, not the create form (keeps the flow short per the plan).
	const clientId = await createClient(page.request, tenant, 'Invoice Test Client');

	const today = new Date().toISOString().slice(0, 10);
	const res = await page.request.post(`/api/t/${tenant}/invoices`, {
		data: {
			clientId,
			payerId: null,
			status: 'draft',
			issueDate: today,
			dueDate: today,
			notes: '',
			lineItems: [
				{
					// No code + no customItemId → not a catalogue line; validation
					// skips the price-list check so no price list is required.
					description: 'Consulting services',
					quantity: 2,
					unitPrice: 100,
					taxable: false
				}
			]
		}
	});
	expect(res.ok(), `seed invoice failed: ${res.status()} ${await res.text()}`).toBeTruthy();
	const inv = await res.json();

	// Navigate to the invoice detail page.
	await page.goto(`/${tenant}/invoices/${inv.id}`);

	// The line-items section renders the total row.
	await expect(page.getByText('Total')).toBeVisible();
	// The header shows the monetary total (200.00 from 2 × 100).
	await expect(page.getByText('$200.00').first()).toBeVisible();

	// The status badge shows "draft" initially. Use .first() since invoice status
	// strings can also appear in filter dropdowns on cached list views.
	await expect(page.getByText('draft').first()).toBeVisible();

	// Advance draft → sent.
	await page.getByRole('button', { name: 'Mark sent' }).click();
	await expect(page.getByText('sent').first()).toBeVisible();

	// Advance sent → paid.
	await page.getByRole('button', { name: 'Mark paid' }).click();
	await expect(page.getByText('paid').first()).toBeVisible();

	// Once paid there is no next-action button; the "Paid ✓" badge renders instead.
	await expect(page.getByText('Paid ✓')).toBeVisible();
	await expect(page.getByRole('button', { name: /Mark/ })).not.toBeVisible();
});

test('invoices list page renders heading and seeded invoice', async ({ page }) => {
	// Seed a client and invoice so the list is non-empty.
	const clientId = await createClient(page.request, tenant, 'List Test Client');
	const today = new Date().toISOString().slice(0, 10);
	const res = await page.request.post(`/api/t/${tenant}/invoices`, {
		data: {
			clientId,
			payerId: null,
			status: 'draft',
			issueDate: today,
			dueDate: today,
			notes: '',
			lineItems: [
				{
					description: 'List item',
					quantity: 1,
					unitPrice: 50,
					taxable: false
				}
			]
		}
	});
	expect(res.ok(), `seed invoice failed: ${res.status()} ${await res.text()}`).toBeTruthy();

	await page.goto(`/${tenant}/invoices`);
	await expect(page.getByRole('heading', { name: 'Invoices' })).toBeVisible();
	// The table shows the client name from the seeded invoice.
	await expect(page.getByText('List Test Client')).toBeVisible();
});
