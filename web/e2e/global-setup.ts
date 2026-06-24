import { request } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname } from 'node:path';
import { BASE_URL, STATE_FILE } from '../playwright.config';
import { signupOwner, createClient } from './fixtures';

// Runs once after the webServer is up: sign up the owner (first-run), seed a
// baseline client, then persist the authenticated session (storageState) plus
// the tenant uuid for the specs to read.
export const TENANT_FILE = 'e2e/.auth/tenant.json';

export default async function globalSetup() {
	const api = await request.newContext({ baseURL: BASE_URL });

	const tenant = await signupOwner(api);
	await createClient(api, tenant, 'Acme Baseline Client');

	await mkdir(dirname(STATE_FILE), { recursive: true });
	await api.storageState({ path: STATE_FILE });
	await writeFile(TENANT_FILE, JSON.stringify({ tenant }));
	await api.dispose();
}
