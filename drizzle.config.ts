import { defineConfig } from 'drizzle-kit';
import { getDbPath } from './src/lib/data-dir.js';

export default defineConfig({
	schema: './src/lib/db/drizzle-schema.ts',
	out: './drizzle',
	dialect: 'sqlite',
	dbCredentials: {
		url: getDbPath()
	}
});
