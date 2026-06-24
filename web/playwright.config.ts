import { defineConfig } from '@playwright/test';

// E2E config: boots the REAL tallyo single binary (Go + embedded SPA + SQLite)
// against a fresh temp data dir, seeds an owner via the API in global-setup, and
// drives the SPA already logged in. Local-only — no CI, no cross-browser.
const PORT = 8099;
export const BASE_URL = `http://localhost:${PORT}`;
export const STATE_FILE = 'e2e/.auth/state.json';

export default defineConfig({
	testDir: 'e2e',
	fullyParallel: false,
	workers: 1,
	globalSetup: './e2e/global-setup.ts',
	use: {
		baseURL: BASE_URL,
		storageState: STATE_FILE,
		trace: 'retain-on-failure'
	},
	webServer: {
		// launch.sh builds the SPA, builds the binary, and runs it on PORT.
		command: `PORT=${PORT} bash e2e/launch.sh`,
		url: BASE_URL,
		reuseExistingServer: false,
		timeout: 180_000
	}
});
