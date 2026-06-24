import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';

// Smoke test: proves the whole harness — binary boot, API seed, logged-in
// storageState, SPA navigation, and an assertion on seeded data.
const { tenant } = JSON.parse(readFileSync('e2e/.auth/tenant.json', 'utf8')) as { tenant: string };

test('logged in, seeded client shows on clients page', async ({ page }) => {
	await page.goto(`/${tenant}/clients`);
	await expect(page.getByText('Acme Baseline Client')).toBeVisible();
});
