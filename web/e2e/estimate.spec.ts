import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { createClient } from './fixtures';

// Estimate lifecycle e2e: create via API, view detail, accept, convert to
// invoice, assert the "View resulting invoice" link renders.
const { tenant } = JSON.parse(readFileSync('e2e/.auth/tenant.json', 'utf8')) as { tenant: string };

test('estimate create → accept → convert to invoice → view resulting invoice link', async ({
	page
}) => {
	// Seed a client and estimate via the API so the test only drives the lifecycle UI.
	const clientId = await createClient(page.request, tenant, 'Estimate Test Client');

	const today = new Date().toISOString().slice(0, 10);
	const res = await page.request.post(`/api/t/${tenant}/estimates`, {
		data: {
			clientId,
			payerId: null,
			status: 'draft',
			issueDate: today,
			validUntil: today,
			notes: '',
			lineItems: [
				{
					// No code + no customItemId → not a catalogue line.
					description: 'Design work',
					quantity: 3,
					unitPrice: 80,
					taxable: false
				}
			]
		}
	});
	expect(res.ok(), `seed estimate failed: ${res.status()} ${await res.text()}`).toBeTruthy();
	const est = await res.json();

	// Navigate to the estimate detail page.
	await page.goto(`/${tenant}/estimates/${est.id}`);

	// The estimate shows the line-items total.
	await expect(page.getByText('Total')).toBeVisible();
	// 3 × 80 = 240
	await expect(page.getByText('$240.00').first()).toBeVisible();

	// Status badge starts as "draft" — .first() guards against the filter
	// dropdown also listing "draft" as an option on any cached list view.
	await expect(page.getByText('draft').first()).toBeVisible();

	// Accept the estimate.
	await page.getByRole('button', { name: 'Accept' }).click();
	await expect(page.getByText('accepted').first()).toBeVisible();

	// Convert to invoice — button is always visible when status !== 'converted'.
	await page.getByRole('button', { name: 'Convert to invoice' }).click();

	// After conversion the badge updates and a link to the new invoice appears.
	// Use .first() since "converted" may also appear as a filter option in the
	// estimates DataTable; here we're on the detail page, so the first match is
	// the status badge in the header.
	await expect(page.getByText('converted').first()).toBeVisible();
	await expect(page.getByRole('link', { name: 'View resulting invoice →' })).toBeVisible();
});

test('estimates list page renders heading', async ({ page }) => {
	await page.goto(`/${tenant}/estimates`);
	await expect(page.getByRole('heading', { name: 'Estimates' })).toBeVisible();
});
