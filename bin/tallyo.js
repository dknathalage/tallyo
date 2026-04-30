#!/usr/bin/env node
import { mkdirSync } from 'node:fs';
import { homedir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { parseArgs } from 'node:util';
import getPort from 'get-port';
import open from 'open';
import Database from 'better-sqlite3';
import { drizzle } from 'drizzle-orm/better-sqlite3';
import { migrate } from 'drizzle-orm/better-sqlite3/migrator';

const __dirname = dirname(fileURLToPath(import.meta.url));
const pkgRoot = resolve(__dirname, '..');

const { values } = parseArgs({
  options: {
    port: { type: 'string' },
    'data-dir': { type: 'string' },
    'no-open': { type: 'boolean', default: false },
    help: { type: 'boolean', short: 'h', default: false },
    version: { type: 'boolean', short: 'v', default: false },
  },
});

if (values.help) {
  console.log(`Usage: tallyo [options]

Options:
  --port <n>           Port to bind (default: first free port from 3000)
  --data-dir <path>    Data directory (default: $DATA_DIR or ~/.tallyo)
  --no-open            Do not auto-open browser
  -h, --help           Show this help
  -v, --version        Show version
`);
  process.exit(0);
}

if (values.version) {
  const { default: pkg } = await import(pathToFileURL(join(pkgRoot, 'package.json')), { with: { type: 'json' } });
  console.log(pkg.version);
  process.exit(0);
}

const dataDir = values['data-dir'] ?? process.env.DATA_DIR ?? join(homedir(), '.tallyo');
mkdirSync(dataDir, { recursive: true });

const requestedPort = values.port ? Number(values.port) : undefined;
const port = await getPort({ port: requestedPort ?? [3000, 3001, 3002, 3003, 3004, 3005] });
if (requestedPort && port !== requestedPort) {
  console.error(`Port ${requestedPort} is in use.`);
  process.exit(1);
}

const dbPath = join(dataDir, 'tallyo.db');
const sqlite = new Database(dbPath);
const db = drizzle(sqlite);
migrate(db, { migrationsFolder: join(pkgRoot, 'drizzle') });
sqlite.close();

process.env.PORT = String(port);
process.env.HOST = '127.0.0.1';
process.env.DATA_DIR = dataDir;
process.env.NODE_ENV ??= 'production';

const url = `http://localhost:${port}`;
console.log(`Tallyo running at ${url}`);
console.log(`Data: ${dataDir}`);

const shutdown = () => process.exit(0);
process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);

await import(pathToFileURL(join(pkgRoot, 'build', 'index.js')).href);

if (!values['no-open']) {
  open(url).catch(() => {});
}
