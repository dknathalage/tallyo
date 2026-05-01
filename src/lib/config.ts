import { readFileSync, writeFileSync, existsSync } from 'fs';
import { getConfigPath } from './data-dir.js';

export interface AppConfig {
	server: {
		port: number;
		host: string;
	};
	app: {
		defaultCurrency: string;
		dateFormat: string;
		locale: string;
	};
}

const defaults: AppConfig = {
	server: {
		port: 3000,
		host: '0.0.0.0'
	},
	app: {
		defaultCurrency: 'USD',
		dateFormat: 'YYYY-MM-DD',
		locale: 'en-US'
	}
};

let _config: AppConfig | null = null;

export function getConfig(): AppConfig {
	if (_config) return _config;

	const configPath = getConfigPath();

	if (existsSync(configPath)) {
		try {
			const raw = JSON.parse(readFileSync(configPath, 'utf-8')) as Partial<AppConfig>;
			_config = {
				server: { ...defaults.server, ...(raw.server ?? {}) },
				app: { ...defaults.app, ...(raw.app ?? {}) }
			};
		} catch {
			_config = { ...defaults };
		}
	} else {
		_config = { ...defaults };
		writeFileSync(configPath, JSON.stringify(_config, null, 2));
	}

	return _config;
}
