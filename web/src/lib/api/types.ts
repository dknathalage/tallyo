export type Role = 'owner' | 'admin' | 'member' | string;

export interface User {
	id: string;
	tenantId: string;
	email: string;
	name: string;
	role: Role;
	isPlatformAdmin: boolean;
	lastLoginAt: string | null;
}

/**
 * One candidate tenant returned with the 409 tenant-disambiguation login response
 * (and by GET /api/auth/session). `id` is the tenant's public UUID.
 */
export interface EmailTenant {
	id: string;
	tenantName: string;
	role: string;
}

export interface SignupInput {
	businessName: string;
	name: string;
	email: string;
	password: string;
}

export interface InviteInfo {
	email: string;
	role: Role;
}

export interface InviteCreated {
	token: string;
	acceptUrl: string;
}

export interface Payer {
	id: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface PayerInput {
	name: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
}

export interface TaxRate {
	id: string;
	name: string;
	rate: number;
	isDefault: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface TaxRateInput {
	name: string;
	rate: number;
	isDefault: boolean;
}

export interface Client {
	id: string;
	name: string;
	reference: string;
	payerId: string | null;
	payerName: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface ClientInput {
	name: string;
	reference: string;
	payerId: string | null;
	email: string;
	phone: string;
	address: string;
	metadata: string;
}

// ---- Catalogue (tenant-owned, per-item copy-on-write versioning) ----

// The catalogue is the single source of reusable priced line templates. `id` is
// the version-row uuid (what a line item pins via `catalogueItemId`); `version`
// and `isCurrent` describe the copy-on-write history. List endpoints return only
// current rows.
export interface CatalogueItem {
	id: string;
	code: string;
	name: string;
	unit: string;
	category: string;
	unitPrice: number;
	taxable: boolean;
	metadata: string;
	version: number;
	isCurrent: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface CatalogueItemInput {
	code: string;
	name: string;
	unit: string;
	category: string;
	unitPrice: number;
	taxable: boolean;
	metadata: string;
}

// ---- Invoice + estimate domain ----

export interface LineItem {
	id: string;
	sessionId: string | null;
	invoiceId: string | null;
	catalogueItemId: string | null;
	code: string;
	description: string;
	serviceDate: string;
	unit: string;
	startTime: string;
	endTime: string;
	quantity: number;
	unitPrice: number;
	taxable: boolean;
	lineTotal: number;
	sortOrder: number;
}

// LineItemInput is the writable subset of a line item (no id/uuid/lineTotal —
// the server's DecodeJSON rejects unknown fields, so only these are sent).
export interface LineItemInput {
	catalogueItemId: string | null;
	code: string;
	description: string;
	serviceDate: string;
	unit: string;
	startTime: string;
	endTime: string;
	quantity: number;
	unitPrice: number;
	taxable: boolean;
	sortOrder: number;
}

export type InvoiceStatus = 'draft' | 'sent' | 'overdue' | 'paid' | string;

export interface Invoice {
	id: string;
	number: string;
	clientId: string;
	clientName: string;
	payerId: string | null;
	status: InvoiceStatus;
	issueDate: string;
	dueDate: string;
	subtotal: number;
	tax: number;
	total: number;
	notes: string;
	businessSnapshot: string;
	clientSnapshot: string;
	payerSnapshot: string;
	createdAt: string;
	updatedAt: string;
	totalPaid: number;
	balance: number;
	lineItems: LineItem[];
}

// The create/update payload: the flat InvoiceInput fields plus line items.
// tax is server-derived; it is intentionally omitted from the payload.
export interface InvoiceInput {
	clientId: string;
	payerId: string | null;
	status: InvoiceStatus;
	issueDate: string;
	dueDate: string;
	notes: string;
	lineItems: LineItemInput[];
}

export interface Payment {
	id: string;
	invoiceId: string;
	amount: number;
	paymentDate: string;
	method: string;
	notes: string;
	createdAt: string;
	updatedAt: string;
}

export interface PaymentInput {
	amount: number;
	paymentDate: string;
	method: string;
	notes: string;
}

export interface ClientStats {
	invoiceCount: number;
	totalInvoiced: number;
	totalPaid: number;
}

export type EstimateStatus = 'draft' | 'accepted' | 'declined' | 'converted' | string;

export type EstimateLineItem = LineItem;
export type EstimateLineItemInput = LineItemInput;

export interface Estimate {
	id: string;
	number: string;
	clientId: string | null;
	clientName: string;
	payerId: string | null;
	status: EstimateStatus;
	issueDate: string;
	validUntil: string;
	subtotal: number;
	tax: number;
	total: number;
	notes: string;
	convertedInvoiceId: string | null;
	businessSnapshot: string;
	clientSnapshot: string;
	payerSnapshot: string;
	createdAt: string;
	updatedAt: string;
	lineItems: EstimateLineItem[];
}

export interface EstimateInput {
	clientId: string;
	payerId: string | null;
	status: EstimateStatus;
	issueDate: string;
	validUntil: string;
	notes: string;
	lineItems: EstimateLineItemInput[];
}

/** One field-level validation failure from the 422 response. */
export interface ValidationDetail {
	line: number;
	field: string;
	message: string;
}

// ---- Sessions (per-client service sessions with a billing lifecycle) ----

export type SessionStatus = 'scheduled' | 'recorded' | 'drafted' | 'sent' | 'paid';

export interface Session {
	id: string;
	clientId: string;
	serviceDate: string;
	note: string;
	tags: string[];
	status: SessionStatus;
	invoiceId: string | null;
	createdAt: string;
	updatedAt: string;
}

export interface SessionInput {
	clientId: string;
	serviceDate: string;
	note: string;
	tags: string[];
	status: SessionStatus;
}

/**
 * A clustered invoice suggestion: a client's recorded-but-unbilled sessions,
 * grouped to draft a single invoice. The backend supplies clientId/ids/
 * from/to/count; the client name and an estimated total are derived in the
 * UI from the loaded clients + sessions.
 */
export interface SessionSuggestion {
	clientId: string;
	ids: string[];
	from: string;
	to: string;
	count: number;
}

/**
 * DataTable server-side query params. `filters` maps a column key to its encoded
 * value — a "contains" string for text, a comma-joined set for enum, or a
 * range key like "start.from" → ISO date. crud.query prefixes each with `f.`.
 */
export interface ListParams {
	sort?: string;
	dir?: 'asc' | 'desc';
	page?: number;
	limit?: number;
	filters?: Record<string, string>;
}

/** One page of a server-side list query plus the unpaginated total. */
export interface ListResult<T> {
	rows: T[];
	total: number;
}
