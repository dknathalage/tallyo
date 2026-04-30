import type { Payment } from '$lib/types/index.js';

export interface PaymentRepository {
	getInvoicePayments(invoiceId: number): Promise<Payment[]>;
	getInvoiceTotalPaid(invoiceId: number): Promise<number>;
	createPayment(data: {
		invoice_id: number;
		amount: number;
		payment_date: string;
		method?: string;
		notes?: string;
	}): Promise<number>;
	deletePayment(id: number): Promise<void>;
}
