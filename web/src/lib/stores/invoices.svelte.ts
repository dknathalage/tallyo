import { createCollectionStore } from './collection.svelte';
import type { Invoice, InvoiceInput } from '$lib/api/types';

// The create/update payload is the invoice input fields plus its line items.
// The generic CRUD sends this object as-is; the server is authoritative on
// totals and snapshots.
export type InvoiceCreatePayload = InvoiceInput;

export const invoices = createCollectionStore<Invoice, InvoiceCreatePayload>('invoices', 'invoice');
