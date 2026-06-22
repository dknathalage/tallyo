export type Role = 'owner' | 'admin' | 'member' | string;

export type Zone = 'national' | 'remote' | 'very_remote';

export type MgmtType = 'plan' | 'self' | string;

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
	zone?: Zone;
}

export interface InviteInfo {
	email: string;
	role: Role;
}

export interface InviteCreated {
	token: string;
	acceptUrl: string;
}

export interface PlanManager {
	id: string;
	name: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface PlanManagerInput {
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

export interface Participant {
	id: string;
	name: string;
	ndisNumber: string;
	planStart: string;
	planEnd: string;
	mgmtType: MgmtType;
	planManagerId: string | null;
	planManagerName: string;
	email: string;
	phone: string;
	address: string;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface ParticipantInput {
	name: string;
	ndisNumber: string;
	planStart: string;
	planEnd: string;
	mgmtType: MgmtType;
	planManagerId: string | null;
	email: string;
	phone: string;
	address: string;
	metadata: string;
}

export interface CustomItem {
	id: string;
	name: string;
	rate: number;
	unit: string;
	gstFree: boolean;
	metadata: string;
	createdAt: string;
	updatedAt: string;
}

export interface CustomItemInput {
	name: string;
	rate: number;
	unit: string;
	gstFree: boolean;
	metadata: string;
}

// ---- Global NDIS Support Catalogue (read-only for tenants) ----

// NOTE: the support-catalogue read endpoints still address catalogue rows by their
// internal int id (catalogue ingest is deferred; these slices were not migrated to
// uuid). Their `id`/`catalogVersionId`/`supportItemId` are numbers; the pinned
// uuid refs that cross onto a line item (LineItem.catalogVersionId, etc.) are
// separate and are uuid strings.
export interface CatalogVersion {
	id: number;
	uuid: string;
	label: string;
	effectiveFrom: string;
	effectiveTo: string;
	sourceFilename: string;
	createdAt: string;
}

export interface SupportItem {
	id: number;
	uuid: string;
	catalogVersionId: number;
	code: string;
	name: string;
	unit: string;
	supportCategory: string;
	registrationGroup: string;
	claimType: string;
	gstFree: boolean;
	metadata: string;
}

export interface SupportItemPrice {
	id: number;
	supportItemId: number;
	zone: Zone;
	priceCap: number | null;
}

// ---- Invoice + estimate domain ----

export interface LineItem {
	id: string;
	shiftId: string | null;
	invoiceId: string | null;
	supportItemId: string | null;
	customItemId: string | null;
	catalogVersionId: string | null;
	code: string;
	description: string;
	serviceDate: string;
	unit: string;
	startTime: string;
	endTime: string;
	quantity: number;
	unitPrice: number;
	gstFree: boolean;
	lineTotal: number;
	sortOrder: number;
}

// LineItemInput is the writable subset of a line item (no id/uuid/lineTotal —
// the server's DecodeJSON rejects unknown fields, so only these are sent).
export interface LineItemInput {
	supportItemId: string | null;
	customItemId: string | null;
	catalogVersionId: string | null;
	code: string;
	description: string;
	serviceDate: string;
	unit: string;
	startTime: string;
	endTime: string;
	quantity: number;
	unitPrice: number;
	gstFree: boolean;
	sortOrder: number;
}

export type InvoiceStatus = 'draft' | 'sent' | 'overdue' | 'paid' | string;

export interface Invoice {
	id: string;
	number: string;
	participantId: string;
	participantName: string;
	planManagerId: string | null;
	status: InvoiceStatus;
	issueDate: string;
	dueDate: string;
	subtotal: number;
	tax: number;
	total: number;
	notes: string;
	businessSnapshot: string;
	participantSnapshot: string;
	planManagerSnapshot: string;
	createdAt: string;
	updatedAt: string;
	totalPaid: number;
	balance: number;
	lineItems: LineItem[];
}

// The create/update payload: the flat InvoiceInput fields plus line items.
// tax is server-derived; it is intentionally omitted from the payload.
export interface InvoiceInput {
	participantId: string;
	planManagerId: string | null;
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

export interface ParticipantStats {
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
	participantId: string | null;
	participantName: string;
	planManagerId: string | null;
	status: EstimateStatus;
	issueDate: string;
	validUntil: string;
	subtotal: number;
	tax: number;
	total: number;
	notes: string;
	convertedInvoiceId: string | null;
	businessSnapshot: string;
	participantSnapshot: string;
	planManagerSnapshot: string;
	createdAt: string;
	updatedAt: string;
	lineItems: EstimateLineItem[];
}

export interface EstimateInput {
	participantId: string;
	planManagerId: string | null;
	status: EstimateStatus;
	issueDate: string;
	validUntil: string;
	notes: string;
	lineItems: EstimateLineItemInput[];
}

export type RecurringFrequency = 'weekly' | 'monthly' | 'quarterly' | string;

export interface RecurringLine {
	supportItemId: string | null;
	customItemId: string | null;
	code: string;
	description: string;
	unit: string;
	quantity: number;
	unitPrice: number;
	gstFree: boolean;
	sortOrder: number;
}

export interface RecurringTemplate {
	id: string;
	participantId: string | null;
	participantName: string;
	planManagerId: string | null;
	name: string;
	frequency: RecurringFrequency;
	nextDue: string;
	lineItems: RecurringLine[];
	taxRate: number;
	notes: string;
	isActive: boolean;
	createdAt: string;
	updatedAt: string;
}

export interface RecurringInput {
	participantId: string | null;
	planManagerId: string | null;
	name: string;
	frequency: RecurringFrequency;
	nextDue: string;
	lineItems: RecurringLine[];
	taxRate: number;
	notes: string;
	isActive: boolean;
}

/** One field-level validation failure from the 422 response. */
export interface ValidationDetail {
	line: number;
	field: string;
	message: string;
}

// ---- Shifts (per-participant service shifts with a billing lifecycle) ----

export type ShiftStatus = 'scheduled' | 'recorded' | 'drafted' | 'sent' | 'paid';

export interface Shift {
	id: string;
	participantId: string;
	serviceDate: string;
	note: string;
	tags: string[];
	status: ShiftStatus;
	invoiceId: string | null;
	createdAt: string;
	updatedAt: string;
}

export interface ShiftInput {
	participantId: string;
	serviceDate: string;
	note: string;
	tags: string[];
	status: ShiftStatus;
}

/**
 * A clustered invoice suggestion: a participant's recorded-but-unbilled shifts,
 * grouped to draft a single invoice. The backend supplies participantId/ids/
 * from/to/count; the participant name and an estimated total are derived in the
 * UI from the loaded participants + shifts.
 */
export interface ShiftSuggestion {
	participantId: string;
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
