import type { Payment } from '$lib/types/index.js';

export interface PaymentRepository {
	getInvoicePayments(invoiceId: number): Payment[];
	getInvoiceTotalPaid(invoiceId: number): number;
	createPayment(data: {
		invoice_id: number;
		amount: number;
		payment_date: string;
		method?: string;
		notes?: string;
	}): Promise<number>;
	deletePayment(id: number): Promise<void>;
}
