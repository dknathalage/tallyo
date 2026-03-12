import {
	getInvoicePayments,
	getInvoiceTotalPaid,
	createPayment,
	deletePayment
} from '$lib/db/queries/payments.js';
import type { PaymentRepository } from '../interfaces/PaymentRepository.js';
import type { Payment } from '$lib/types/index.js';

export class SqlitePaymentRepository implements PaymentRepository {
	getInvoicePayments(invoiceId: number): Payment[] {
		return getInvoicePayments(invoiceId);
	}

	getInvoiceTotalPaid(invoiceId: number): number {
		return getInvoiceTotalPaid(invoiceId);
	}

	createPayment(data: {
		invoice_id: number;
		amount: number;
		payment_date: string;
		method?: string;
		notes?: string;
	}): Promise<number> {
		return createPayment(data);
	}

	deletePayment(id: number): Promise<void> {
		return deletePayment(id);
	}
}
