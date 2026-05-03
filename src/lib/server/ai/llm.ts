import { mkdir, stat } from 'node:fs/promises';
import { createWriteStream } from 'node:fs';
import { join } from 'node:path';
import { getLlama, LlamaChatSession, defineChatSessionFunction } from 'node-llama-cpp';
import type { Llama, LlamaModel, LlamaContext } from 'node-llama-cpp';
import { log } from '../logger.js';

const l = log('ai:llm');

const MODEL_URL =
	'https://huggingface.co/bartowski/Qwen2.5-7B-Instruct-GGUF/resolve/main/Qwen2.5-7B-Instruct-Q4_K_M.gguf?download=true';
const MODEL_FILE = 'Qwen2.5-7B-Instruct-Q4_K_M.gguf';
const MAX_DOWNLOAD_BYTES = 8 * 1024 * 1024 * 1024;

export type AiProgressEvent =
	| { type: 'download_start'; total: number }
	| { type: 'download_progress'; bytes: number; total: number }
	| { type: 'download_complete' }
	| { type: 'model_loading' }
	| { type: 'model_ready' }
	| { type: 'generating' };

export type AiProgressFn = (event: AiProgressEvent) => void;

let llamaPromise: Promise<Llama> | null = null;
let modelPromise: Promise<LlamaModel> | null = null;

function modelDir(): string {
	const base = process.env['DATA_DIR'];
	if (!base) throw new Error('DATA_DIR not set');
	return join(base, 'models');
}

async function downloadModelFile(path: string, onProgress?: AiProgressFn): Promise<void> {
	l.info('model download start', { path, url: MODEL_URL });
	const res = await fetch(MODEL_URL);
	if (!res.ok || !res.body) {
		l.error('model download failed', { status: res.status });
		throw new Error(`Model download failed: ${res.status}`);
	}
	const total = Number(res.headers.get('content-length') ?? '0');
	if (total > MAX_DOWNLOAD_BYTES) throw new Error('Model file too large');
	l.info('model download started', { total });
	onProgress?.({ type: 'download_start', total });

	const out = createWriteStream(path);
	const reader = res.body.getReader();
	let bytes = 0;
	let lastEmit = 0;
	const EMIT_EVERY = 1024 * 1024;

	for (let i = 0; i < 1_000_000; i++) {
		const { done, value } = await reader.read();
		if (done) break;
		await new Promise<void>((resolve, reject) => {
			out.write(value, (err) => (err ? reject(err) : resolve()));
		});
		bytes += value.byteLength;
		if (bytes - lastEmit >= EMIT_EVERY || total > 0) {
			if (bytes - lastEmit >= EMIT_EVERY) {
				onProgress?.({ type: 'download_progress', bytes, total });
				lastEmit = bytes;
			}
		}
	}
	await new Promise<void>((resolve, reject) => out.end((err: unknown) => (err ? reject(err as Error) : resolve())));
	l.info('model download complete', { bytes });
	onProgress?.({ type: 'download_complete' });
}

async function ensureModelFile(onProgress?: AiProgressFn): Promise<string> {
	const dir = modelDir();
	await mkdir(dir, { recursive: true });
	const path = join(dir, MODEL_FILE);
	try {
		const s = await stat(path);
		if (s.size > 100_000_000) return path;
	} catch {
		// not present, fall through
	}
	await downloadModelFile(path, onProgress);
	return path;
}

async function getModel(onProgress?: AiProgressFn): Promise<LlamaModel> {
	if (modelPromise) return modelPromise;
	modelPromise = (async () => {
		llamaPromise ??= getLlama();
		const llama = await llamaPromise;
		const path = await ensureModelFile(onProgress);
		l.info('model loading', { path });
		onProgress?.({ type: 'model_loading' });
		const model = await llama.loadModel({ modelPath: path });
		l.info('model ready');
		onProgress?.({ type: 'model_ready' });
		return model;
	})();
	try {
		return await modelPromise;
	} catch (err) {
		l.error('model load failed', { error: err instanceof Error ? err.message : String(err) });
		modelPromise = null;
		throw err;
	}
}

export async function createSession(
	systemPrompt: string,
	onProgress?: AiProgressFn
): Promise<{ session: LlamaChatSession; context: LlamaContext }> {
	const model = await getModel(onProgress);
	l.debug('creating context', { contextSize: 4096, sequences: 4 });
	const context = await model.createContext({ contextSize: 4096, sequences: 4 });
	const session = new LlamaChatSession({
		contextSequence: context.getSequence(),
		systemPrompt
	});
	return { session, context };
}

export { defineChatSessionFunction };
