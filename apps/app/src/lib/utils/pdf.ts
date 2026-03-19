import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { Invoice, LineItem, Estimate, EstimateLineItem } from '$lib/types';
import { i18n } from '$lib/stores/i18n.svelte.js';
import { parseSnapshot } from '$lib/utils/snapshot.js';

function formatPdfCurrencyWithCode(amount: number, currencyCode: string = 'USD'): string {
	return new Intl.NumberFormat('en-US', {
		style: 'currency',
		currency: currencyCode
	}).format(amount);
}

interface PdfDocumentData {
	title: string;
	number: string;
	clientName: string | undefined;
	date: string;
	secondDateLabel: string;
	secondDate: string;
	status: string;
	currencyCode: string;
	subtotal: number;
	taxRate: number;
	taxAmount: number;
	total: number;
	notes: string;
	businessSnapshot: string;
	clientSnapshot: string;
	payerSnapshot: string;
	lineItems: Array<{ description: string; quantity: number; rate: number; amount: number; notes: string }>;
	fileName: string;
}

function renderPdf(data: PdfDocumentData): void {
	const doc = new jsPDF();
	const pageWidth = doc.internal.pageSize.getWidth();
	let y = 20;

	const business = parseSnapshot(data.businessSnapshot);
	const client = parseSnapshot(data.clientSnapshot);
	const payer = parseSnapshot(data.payerSnapshot);

	// --- Header with logo ---
	let headerTextX = 14;

	if (business.logo) {
		try {
			doc.addImage(business.logo, 'AUTO', 14, y - 5, 20, 20);
			headerTextX = 38;
		} catch {
			// Skip logo if it fails to load
		}
	}

	if (business.name) {
		doc.setFontSize(16);
		doc.setTextColor(17, 24, 39);
		doc.text(business.name, headerTextX, y);
		y += 6;

		doc.setFontSize(9);
		doc.setTextColor(107, 114, 128);
		if (business.address) {
			const addressLines = business.address.split('\n');
			for (const line of addressLines) {
				doc.text(line, headerTextX, y);
				y += 4;
			}
		}
		// Business metadata (e.g., ABN)
		for (const [key, value] of Object.entries(business.metadata)) {
			doc.text(`${key}: ${value}`, headerTextX, y);
			y += 4;
		}
	}

	// Document title on the right
	doc.setFontSize(28);
	doc.setTextColor(37, 99, 235);
	doc.text(data.title, pageWidth - 14, 20, { align: 'right' });

	doc.setFontSize(12);
	doc.setTextColor(107, 114, 128);
	doc.text(data.number, pageWidth - 14, 28, { align: 'right' });

	y = Math.max(y, 40);
	y += 4;

	// Divider
	doc.setDrawColor(229, 231, 235);
	doc.setLineWidth(0.5);
	doc.line(14, y, pageWidth - 14, y);
	y += 10;

	// --- Service For / Bill To sections side by side ---
	const hasPayer = payer.name.trim().length > 0;
	const colWidth = hasPayer ? (pageWidth - 28) / 2 : pageWidth - 28;

	// SERVICE FOR (left)
	const serviceForY = y;
	doc.setFontSize(9);
	doc.setTextColor(107, 114, 128);
	doc.text(i18n.t('pdf.serviceFor'), 14, y);
	y += 6;

	doc.setFontSize(11);
	doc.setTextColor(17, 24, 39);
	doc.text(client.name || data.clientName || i18n.t('common.unknown'), 14, y);
	y += 5;

	doc.setFontSize(9);
	doc.setTextColor(107, 114, 128);
	if (client.email) { doc.text(client.email, 14, y); y += 4; }
	if (client.phone) { doc.text(client.phone, 14, y); y += 4; }
	if (client.address) {
		const lines = client.address.split('\n');
		for (const line of lines) { doc.text(line, 14, y); y += 4; }
	}
	for (const [key, value] of Object.entries(client.metadata)) {
		doc.text(`${key}: ${value}`, 14, y);
		y += 4;
	}

	// BILL TO (right) if payer exists
	let rightY = serviceForY;
	if (hasPayer) {
		const rightX = 14 + colWidth + 4;
		doc.setFontSize(9);
		doc.setTextColor(107, 114, 128);
		doc.text(i18n.t('pdf.billTo'), rightX, rightY);
		rightY += 6;

		doc.setFontSize(11);
		doc.setTextColor(17, 24, 39);
		doc.text(payer.name, rightX, rightY);
		rightY += 5;

		doc.setFontSize(9);
		doc.setTextColor(107, 114, 128);
		if (payer.email) { doc.text(payer.email, rightX, rightY); rightY += 4; }
		if (payer.phone) { doc.text(payer.phone, rightX, rightY); rightY += 4; }
		if (payer.address) {
			const lines = payer.address.split('\n');
			for (const line of lines) { doc.text(line, rightX, rightY); rightY += 4; }
		}
		for (const [key, value] of Object.entries(payer.metadata)) {
			doc.text(`${key}: ${value}`, rightX, rightY);
			rightY += 4;
		}
	}

	y = Math.max(y, rightY) + 6;

	// --- Details row ---
	doc.setFontSize(9);
	doc.setTextColor(107, 114, 128);
	const detailsY = y;

	const numberLabel = data.title === i18n.t('pdf.estimate') ? i18n.t('pdf.estimateNumber') : i18n.t('pdf.invoiceNumber');
	doc.text(numberLabel, 14, detailsY);
	doc.setTextColor(17, 24, 39);
	doc.text(data.number, 40, detailsY);

	doc.setTextColor(107, 114, 128);
	doc.text(i18n.t('pdf.date'), 80, detailsY);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfDate(data.date), 96, detailsY);

	doc.setTextColor(107, 114, 128);
	doc.text(data.secondDateLabel, 130, detailsY);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfDate(data.secondDate), 152, detailsY);

	doc.setTextColor(107, 114, 128);
	doc.text(i18n.t('pdf.status'), 170, detailsY);
	doc.setTextColor(17, 24, 39);
	doc.text(data.status.charAt(0).toUpperCase() + data.status.slice(1), pageWidth - 14, detailsY, { align: 'right' });

	y = detailsY + 10;

	const cc = data.currencyCode || 'USD';

	// --- Line items table ---
	const tableBody = data.lineItems.map((item) => [
		item.notes ? `${item.description}\n${item.notes}` : item.description,
		String(item.quantity),
		formatPdfCurrencyWithCode(item.rate, cc),
		formatPdfCurrencyWithCode(item.amount, cc)
	]);

	autoTable(doc, {
		startY: y,
		head: [[i18n.t('pdf.description'), i18n.t('pdf.quantity'), i18n.t('pdf.rate'), i18n.t('pdf.amount')]],
		body: tableBody,
		theme: 'striped',
		headStyles: {
			fillColor: [37, 99, 235],
			textColor: [255, 255, 255],
			fontStyle: 'bold',
			fontSize: 10
		},
		bodyStyles: {
			fontSize: 10,
			textColor: [17, 24, 39]
		},
		columnStyles: {
			0: { cellWidth: 'auto' },
			1: { halign: 'right', cellWidth: 25 },
			2: { halign: 'right', cellWidth: 30 },
			3: { halign: 'right', cellWidth: 30 }
		},
		margin: { left: 14, right: 14 }
	});

	// Summary section
	y = (doc as any).lastAutoTable.finalY + 10;
	const summaryX = pageWidth - 80;
	const summaryValueX = pageWidth - 14;

	doc.setFontSize(10);
	doc.setTextColor(107, 114, 128);
	doc.text(i18n.t('pdf.subtotal'), summaryX, y);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfCurrencyWithCode(data.subtotal, cc), summaryValueX, y, { align: 'right' });

	y += 7;

	doc.setTextColor(107, 114, 128);
	doc.text(i18n.t('pdf.tax', { rate: data.taxRate }), summaryX, y);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfCurrencyWithCode(data.taxAmount, cc), summaryValueX, y, { align: 'right' });

	y += 3;
	doc.setDrawColor(209, 213, 219);
	doc.line(summaryX, y, summaryValueX, y);

	y += 7;

	doc.setFontSize(12);
	doc.setTextColor(17, 24, 39);
	doc.setFont(undefined!, 'bold');
	doc.text(i18n.t('pdf.total'), summaryX, y);
	doc.text(formatPdfCurrencyWithCode(data.total, cc), summaryValueX, y, { align: 'right' });
	doc.setFont(undefined!, 'normal');

	// Notes
	if (data.notes) {
		y += 16;
		doc.setFontSize(9);
		doc.setTextColor(107, 114, 128);
		doc.text(i18n.t('pdf.notes'), 14, y);
		y += 6;
		doc.setFontSize(10);
		doc.setTextColor(55, 65, 81);
		const noteLines = doc.splitTextToSize(data.notes, pageWidth - 28);
		doc.text(noteLines, 14, y);
	}

	// Footer
	const footerY = doc.internal.pageSize.getHeight() - 20;
	doc.setFontSize(10);
	doc.setTextColor(156, 163, 175);
	doc.text(i18n.t('pdf.thankYou'), pageWidth / 2, footerY, { align: 'center' });

	doc.save(data.fileName);
}

export function exportInvoicePdf(invoice: Invoice, lineItems: LineItem[]): void {
	renderPdf({
		title: i18n.t('pdf.invoice'),
		number: invoice.invoice_number,
		clientName: invoice.client_name,
		date: invoice.date,
		secondDateLabel: i18n.t('pdf.due'),
		secondDate: invoice.due_date,
		status: invoice.status,
		currencyCode: invoice.currency_code || 'USD',
		subtotal: invoice.subtotal,
		taxRate: invoice.tax_rate,
		taxAmount: invoice.tax_amount,
		total: invoice.total,
		notes: invoice.notes,
		businessSnapshot: invoice.business_snapshot,
		clientSnapshot: invoice.client_snapshot,
		payerSnapshot: invoice.payer_snapshot,
		lineItems: lineItems.map((item) => ({
			description: item.description,
			quantity: item.quantity,
			rate: item.rate,
			amount: item.amount,
			notes: item.notes
		})),
		fileName: `invoice-${invoice.invoice_number}.pdf`
	});
}

export function exportEstimatePdf(estimate: Estimate, lineItems: EstimateLineItem[]): void {
	renderPdf({
		title: i18n.t('pdf.estimate'),
		number: estimate.estimate_number,
		clientName: estimate.client_name,
		date: estimate.date,
		secondDateLabel: i18n.t('pdf.validUntil'),
		secondDate: estimate.valid_until,
		status: estimate.status,
		currencyCode: estimate.currency_code || 'USD',
		subtotal: estimate.subtotal,
		taxRate: estimate.tax_rate,
		taxAmount: estimate.tax_amount,
		total: estimate.total,
		notes: estimate.notes,
		businessSnapshot: estimate.business_snapshot,
		clientSnapshot: estimate.client_snapshot,
		payerSnapshot: estimate.payer_snapshot,
		lineItems: lineItems.map((item) => ({
			description: item.description,
			quantity: item.quantity,
			rate: item.rate,
			amount: item.amount,
			notes: item.notes
		})),
		fileName: `estimate-${estimate.estimate_number}.pdf`
	});
}

function formatPdfDate(dateStr: string): string {
	const date = new Date(dateStr + 'T00:00:00');
	return new Intl.DateTimeFormat('en-US', {
		month: 'short',
		day: 'numeric',
		year: 'numeric'
	}).format(date);
}
