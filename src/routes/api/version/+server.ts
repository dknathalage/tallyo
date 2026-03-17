import { json } from '@sveltejs/kit';
import { execSync } from 'child_process';

export function GET() {
	let sha = 'unknown';
	try {
		sha = execSync('git rev-parse --short HEAD', { encoding: 'utf-8' }).trim();
	} catch {
		// not a git repo in production — that's fine
	}
	return json({ version: sha });
}
