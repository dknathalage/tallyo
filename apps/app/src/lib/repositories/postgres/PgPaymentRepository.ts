import {
	getInvoicePayments,
	getInvoiceTotalPaid,
	createPayment,
	deletePayment
} from '$lib/db/queries/payments.js';
import type { PaymentRepository } from '../interfaces/PaymentRepository.js';
import type { Payment } from '$lib/types/index.js';

export class PgPaymentRepository implements PaymentRepository {
	async getInvoicePayments(invoiceId: number): Promise<Payment[]> {
		return await getInvoicePayments(invoiceId);
	}

	async getInvoiceTotalPaid(invoiceId: number): Promise<number> {
		return await getInvoiceTotalPaid(invoiceId);
	}

	async createPayment(data: {
		invoice_id: number;
		amount: number;
		payment_date: string;
		method?: string;
		notes?: string;
	}): Promise<number> {
		return await createPayment(data);
	}

	async deletePayment(id: number): Promise<void> {
		return await deletePayment(id);
	}
}
