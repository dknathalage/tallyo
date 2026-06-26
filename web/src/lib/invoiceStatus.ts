export type StoredInvoiceStatus = 'draft' | 'sent' | 'paid' | string;
export type EffectiveInvoiceStatus = StoredInvoiceStatus | 'overdue';

function todayYMD(): string {
	const d = new Date();
	const y = d.getFullYear();
	const m = String(d.getMonth() + 1).padStart(2, '0');
	const day = String(d.getDate()).padStart(2, '0');
	return `${y}-${m}-${day}`;
}

/** A 'sent' invoice past its due date (compared by YYYY-MM-DD prefix). Blank dueDate is not overdue. */
export function isOverdue(status: string, dueDate: string | null | undefined): boolean {
	if (status !== 'sent') return false;
	if (!dueDate) return false;
	return dueDate.slice(0, 10) < todayYMD();
}

/** Status to DISPLAY: stored status, except a past-due 'sent' surfaces as 'overdue'. Never persisted. */
export function effectiveStatus(
	status: string,
	dueDate: string | null | undefined
): EffectiveInvoiceStatus {
	return isOverdue(status, dueDate) ? 'overdue' : status;
}
