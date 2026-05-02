const { app, BrowserWindow, shell } = require('electron');
const { join } = require('node:path');
const { mkdirSync } = require('node:fs');
const { pathToFileURL } = require('node:url');

async function boot() {
  const getPort = (await import('get-port')).default;
  const Database = require('better-sqlite3');
  const { drizzle } = require('drizzle-orm/better-sqlite3');
  const { migrate } = require('drizzle-orm/better-sqlite3/migrator');

  const dataDir = process.env.DATA_DIR ?? app.getPath('userData');
  mkdirSync(dataDir, { recursive: true });

  const resBase = app.isPackaged
    ? join(process.resourcesPath, 'app.asar.unpacked')
    : join(__dirname, '..');
  const migrationsDir = join(resBase, 'drizzle');
  const buildEntry = join(resBase, 'build', 'index.js');

  const dbPath = join(dataDir, 'tallyo.db');
  const sqlite = new Database(dbPath);
  migrate(drizzle(sqlite), { migrationsFolder: migrationsDir });
  sqlite.close();

  const port = await getPort({ port: [3000, 3001, 3002, 3003, 3004, 3005] });
  process.env.PORT = String(port);
  process.env.HOST = '127.0.0.1';
  process.env.DATA_DIR = dataDir;
  process.env.TALLYO_MIGRATIONS_DIR = migrationsDir;
  process.env.NODE_ENV ??= 'production';

  await import(pathToFileURL(buildEntry).href);

  const win = new BrowserWindow({
    width: 1280,
    height: 800,
    title: 'Tallyo',
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url);
    return { action: 'deny' };
  });

  await win.loadURL(`http://127.0.0.1:${port}`);
}

app.whenReady().then(boot).catch((err) => {
  console.error('Tallyo failed to boot:', err);
  app.exit(1);
});

app.on('window-all-closed', () => app.quit());
