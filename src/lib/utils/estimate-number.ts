/**
 * Client-side placeholder estimate number generator.
 * The server-side validates and assigns the real number on save.
 */
export function generateEstimateNumber(): string {
	const now = new Date();
	const year = now.getFullYear();
	const month = String(now.getMonth() + 1).padStart(2, '0');
	const day = String(now.getDate()).padStart(2, '0');
	const unique = String(Date.now()).slice(-4);
	return `EST-${year}${month}${day}-${unique}`;
}
