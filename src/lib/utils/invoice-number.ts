/**
 * Invoice number generator using the database to find the next sequential number.
 * Only import from server-side code (+page.server.ts, +server.ts, query files).
 */
export { generateInvoiceNumber } from '../db/number-generators.js';
