/**
 * Client-side placeholder invoice number generator.
 * The server-side will validate and override with the real next number on save.
 * On the server (query functions), use generateInvoiceNumberFromDb() directly.
 */

// Server-side generator - only import this from server-side code
export function generateInvoiceNumberServer(): string {
	// This function is only called from server-side query functions
	// where better-sqlite3 is available. Import happens dynamically.
	throw new Error('Use generateInvoiceNumber() from query context instead');
}

/**
 * Generates a timestamp-based placeholder number for new invoice forms.
 * The actual unique number is confirmed server-side on creation.
 */
export function generateInvoiceNumber(): string {
	const now = new Date();
	const year = now.getFullYear();
	const month = String(now.getMonth() + 1).padStart(2, '0');
	const day = String(now.getDate()).padStart(2, '0');
	// Use timestamp for uniqueness in the form
	const unique = String(Date.now()).slice(-4);
	return `INV-${year}${month}${day}-${unique}`;
}
