import { json } from '@sveltejs/kit';
import { readFileSync } from 'fs';
import { resolve } from 'path';

export function GET() {
	const pkg = JSON.parse(readFileSync(resolve('package.json'), 'utf-8'));
	return json({ version: pkg.version });
}
