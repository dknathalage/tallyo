import { getIO } from '$lib/io/index.js';

export async function downloadCsv(csvString: string, filename: string): Promise<void> {
	const io = await getIO();
	const blob = new Blob([csvString], { type: 'text/csv;charset=utf-8;' });
	await io.exportBlob(blob, filename, 'text/csv');
}
