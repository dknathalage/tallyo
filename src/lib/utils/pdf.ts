import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { Invoice, LineItem, Estimate, EstimateLineItem } from '$lib/types';
import { i18n } from '$lib/stores/i18n.svelte.js';
import { parseSnapshot } from '$lib/utils/snapshot.js';

function formatPdfCurrencyWithCode(amount: number, currencyCode = 'USD'): string {
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
	lineItems: { description: string; quantity: number; rate: number; amount: number; notes: string }[];
	fileName: string;
}

function renderBusinessLogo(
	doc: jsPDF,
	business: ReturnType<typeof parseSnapshot>,
	y: number
): number {
	if (business.logo === undefined || business.logo === '') return 14;
	try {
		doc.addImage(business.logo, 'AUTO', 14, y - 5, 20, 20);
		return 38;
	} catch {
		return 14;
	}
}

function renderBusinessBlock(
	doc: jsPDF,
	business: ReturnType<typeof parseSnapshot>,
	headerTextX: number,
	startY: number
): number {
	let y = startY;
	if (!business.name) return y;
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
	for (const [key, value] of Object.entries(business.metadata)) {
		doc.text(`${key}: ${value}`, headerTextX, y);
		y += 4;
	}
	return y;
}

function renderHeader(
	doc: jsPDF,
	data: PdfDocumentData,
	business: ReturnType<typeof parseSnapshot>
): number {
	const pageWidth = doc.internal.pageSize.getWidth();
	const headerTextX = renderBusinessLogo(doc, business, 20);
	let y = renderBusinessBlock(doc, business, headerTextX, 20);

	doc.setFontSize(28);
	doc.setTextColor(37, 99, 235);
	doc.text(data.title, pageWidth - 14, 20, { align: 'right' });

	doc.setFontSize(12);
	doc.setTextColor(107, 114, 128);
	doc.text(data.number, pageWidth - 14, 28, { align: 'right' });

	y = Math.max(y, 40) + 4;

	doc.setDrawColor(229, 231, 235);
	doc.setLineWidth(0.5);
	doc.line(14, y, pageWidth - 14, y);
	return y + 10;
}

interface PartyRenderOpts {
	party: ReturnType<typeof parseSnapshot>;
	x: number;
	startY: number;
	heading: string;
	displayName: string;
}

function renderParty(doc: jsPDF, opts: PartyRenderOpts): number {
	const { party, x, heading, displayName } = opts;
	let y = opts.startY;
	doc.setFontSize(9);
	doc.setTextColor(107, 114, 128);
	doc.text(heading, x, y);
	y += 6;

	doc.setFontSize(11);
	doc.setTextColor(17, 24, 39);
	doc.text(displayName, x, y);
	y += 5;

	doc.setFontSize(9);
	doc.setTextColor(107, 114, 128);
	if (party.email) { doc.text(party.email, x, y); y += 4; }
	if (party.phone) { doc.text(party.phone, x, y); y += 4; }
	if (party.address) {
		const lines = party.address.split('\n');
		for (const line of lines) { doc.text(line, x, y); y += 4; }
	}
	for (const [key, value] of Object.entries(party.metadata)) {
		doc.text(`${key}: ${value}`, x, y);
		y += 4;
	}
	return y;
}

interface PartiesArgs {
	data: PdfDocumentData;
	client: ReturnType<typeof parseSnapshot>;
	payer: ReturnType<typeof parseSnapshot>;
	startY: number;
}

function renderParties(doc: jsPDF, args: PartiesArgs): number {
	const { data, client, payer, startY } = args;
	const pageWidth = doc.internal.pageSize.getWidth();
	const hasPayer = payer.name.trim().length > 0;
	const colWidth = hasPayer ? (pageWidth - 28) / 2 : pageWidth - 28;

	const clientName = client.name !== '' ? client.name : (data.clientName ?? i18n.t('common.unknown'));
	const leftY = renderParty(doc, {
		party: client,
		x: 14,
		startY,
		heading: i18n.t('pdf.serviceFor'),
		displayName: clientName
	});

	let rightY = startY;
	if (hasPayer) {
		const rightX = 14 + colWidth + 4;
		rightY = renderParty(doc, {
			party: payer,
			x: rightX,
			startY,
			heading: i18n.t('pdf.billTo'),
			displayName: payer.name
		});
	}

	return Math.max(leftY, rightY) + 6;
}

function renderDetailsRow(doc: jsPDF, data: PdfDocumentData, y: number): number {
	const pageWidth = doc.internal.pageSize.getWidth();
	doc.setFontSize(9);
	doc.setTextColor(107, 114, 128);

	const numberLabel = data.title === i18n.t('pdf.estimate') ? i18n.t('pdf.estimateNumber') : i18n.t('pdf.invoiceNumber');
	doc.text(numberLabel, 14, y);
	doc.setTextColor(17, 24, 39);
	doc.text(data.number, 40, y);

	doc.setTextColor(107, 114, 128);
	doc.text(i18n.t('pdf.date'), 80, y);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfDate(data.date), 96, y);

	doc.setTextColor(107, 114, 128);
	doc.text(data.secondDateLabel, 130, y);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfDate(data.secondDate), 152, y);

	doc.setTextColor(107, 114, 128);
	doc.text(i18n.t('pdf.status'), 170, y);
	doc.setTextColor(17, 24, 39);
	doc.text(data.status.charAt(0).toUpperCase() + data.status.slice(1), pageWidth - 14, y, { align: 'right' });

	return y + 10;
}

function renderLineItemsTable(doc: jsPDF, data: PdfDocumentData, startY: number, cc: string): number {
	const tableBody = data.lineItems.map((item) => [
		item.notes ? `${item.description}\n${item.notes}` : item.description,
		String(item.quantity),
		formatPdfCurrencyWithCode(item.rate, cc),
		formatPdfCurrencyWithCode(item.amount, cc)
	]);

	autoTable(doc, {
		startY,
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

	const finalY = (doc as unknown as { lastAutoTable?: { finalY?: number } }).lastAutoTable?.finalY;
	if (finalY === undefined) {
		throw new Error('autoTable did not produce a finalY value');
	}
	return finalY + 10;
}

function renderSummary(doc: jsPDF, data: PdfDocumentData, startY: number, cc: string): number {
	const pageWidth = doc.internal.pageSize.getWidth();
	let y = startY;
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
	// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- jsPDF setFont requires fontName arg; passing undefined keeps current font per its API
	doc.setFont(undefined!, 'bold');
	doc.text(i18n.t('pdf.total'), summaryX, y);
	doc.text(formatPdfCurrencyWithCode(data.total, cc), summaryValueX, y, { align: 'right' });
	// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- jsPDF setFont requires fontName arg; passing undefined keeps current font per its API
	doc.setFont(undefined!, 'normal');

	return y;
}

function renderNotesAndFooter(doc: jsPDF, data: PdfDocumentData, startY: number): void {
	const pageWidth = doc.internal.pageSize.getWidth();
	let y = startY;
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

	const footerY = doc.internal.pageSize.getHeight() - 20;
	doc.setFontSize(10);
	doc.setTextColor(156, 163, 175);
	doc.text(i18n.t('pdf.thankYou'), pageWidth / 2, footerY, { align: 'center' });
}

function renderPdf(data: PdfDocumentData): void {
	const doc = new jsPDF();
	const business = parseSnapshot(data.businessSnapshot);
	const client = parseSnapshot(data.clientSnapshot);
	const payer = parseSnapshot(data.payerSnapshot);

	let y = renderHeader(doc, data, business);
	y = renderParties(doc, { data, client, payer, startY: y });
	y = renderDetailsRow(doc, data, y);

	const cc = data.currencyCode !== '' ? data.currencyCode : 'USD';
	y = renderLineItemsTable(doc, data, y, cc);
	y = renderSummary(doc, data, y, cc);
	renderNotesAndFooter(doc, data, y);

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
