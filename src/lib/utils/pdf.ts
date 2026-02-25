import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { Invoice, LineItem, PartySnapshot } from '$lib/types';
import { getIO } from '$lib/io/index.js';

function parseSnapshot(json: string): PartySnapshot {
	try {
		const parsed = JSON.parse(json || '{}');
		return {
			name: parsed.name || '',
			email: parsed.email || '',
			phone: parsed.phone || '',
			address: parsed.address || '',
			logo: parsed.logo,
			metadata: parsed.metadata || {}
		};
	} catch {
		return { name: '', email: '', phone: '', address: '', metadata: {} };
	}
}

export async function exportInvoicePdf(invoice: Invoice, lineItems: LineItem[]): Promise<void> {
	const doc = new jsPDF();
	const pageWidth = doc.internal.pageSize.getWidth();
	let y = 20;

	const business = parseSnapshot(invoice.business_snapshot);
	const client = parseSnapshot(invoice.client_snapshot);
	const payer = parseSnapshot(invoice.payer_snapshot);

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

	// Invoice title on the right
	doc.setFontSize(28);
	doc.setTextColor(37, 99, 235);
	doc.text('INVOICE', pageWidth - 14, 20, { align: 'right' });

	doc.setFontSize(12);
	doc.setTextColor(107, 114, 128);
	doc.text(invoice.invoice_number, pageWidth - 14, 28, { align: 'right' });

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
	doc.text('SERVICE FOR', 14, y);
	y += 6;

	doc.setFontSize(11);
	doc.setTextColor(17, 24, 39);
	doc.text(client.name || invoice.client_name || 'Unknown', 14, y);
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
		doc.text('BILL TO', rightX, rightY);
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

	// --- Invoice details row ---
	doc.setFontSize(9);
	doc.setTextColor(107, 114, 128);
	const detailsY = y;

	doc.text('Invoice #:', 14, detailsY);
	doc.setTextColor(17, 24, 39);
	doc.text(invoice.invoice_number, 40, detailsY);

	doc.setTextColor(107, 114, 128);
	doc.text('Date:', 80, detailsY);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfDate(invoice.date), 96, detailsY);

	doc.setTextColor(107, 114, 128);
	doc.text('Due:', 130, detailsY);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfDate(invoice.due_date), 142, detailsY);

	doc.setTextColor(107, 114, 128);
	doc.text('Status:', 170, detailsY);
	doc.setTextColor(17, 24, 39);
	doc.text(invoice.status.charAt(0).toUpperCase() + invoice.status.slice(1), pageWidth - 14, detailsY, { align: 'right' });

	y = detailsY + 10;

	// --- Line items table ---
	const tableBody = lineItems.map((item) => [
		item.notes ? `${item.description}\n${item.notes}` : item.description,
		String(item.quantity),
		formatPdfCurrency(item.rate),
		formatPdfCurrency(item.amount)
	]);

	autoTable(doc, {
		startY: y,
		head: [['Description', 'Quantity', 'Rate', 'Amount']],
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
	doc.text('Subtotal:', summaryX, y);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfCurrency(invoice.subtotal), summaryValueX, y, { align: 'right' });

	y += 7;

	doc.setTextColor(107, 114, 128);
	doc.text(`Tax (${invoice.tax_rate}%):`, summaryX, y);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfCurrency(invoice.tax_amount), summaryValueX, y, { align: 'right' });

	y += 3;
	doc.setDrawColor(209, 213, 219);
	doc.line(summaryX, y, summaryValueX, y);

	y += 7;

	doc.setFontSize(12);
	doc.setTextColor(17, 24, 39);
	doc.setFont(undefined!, 'bold');
	doc.text('Total:', summaryX, y);
	doc.text(formatPdfCurrency(invoice.total), summaryValueX, y, { align: 'right' });
	doc.setFont(undefined!, 'normal');

	// Notes
	if (invoice.notes) {
		y += 16;
		doc.setFontSize(9);
		doc.setTextColor(107, 114, 128);
		doc.text('NOTES', 14, y);
		y += 6;
		doc.setFontSize(10);
		doc.setTextColor(55, 65, 81);
		const noteLines = doc.splitTextToSize(invoice.notes, pageWidth - 28);
		doc.text(noteLines, 14, y);
	}

	// Footer
	const footerY = doc.internal.pageSize.getHeight() - 20;
	doc.setFontSize(10);
	doc.setTextColor(156, 163, 175);
	doc.text('Thank you for your business', pageWidth / 2, footerY, { align: 'center' });

	const io = await getIO();
	const pdfBlob = doc.output('blob');
	await io.exportBlob(pdfBlob, `invoice-${invoice.invoice_number}.pdf`, 'application/pdf');
}

function formatPdfCurrency(amount: number): string {
	return new Intl.NumberFormat('en-US', {
		style: 'currency',
		currency: 'USD'
	}).format(amount);
}

function formatPdfDate(dateStr: string): string {
	const date = new Date(dateStr + 'T00:00:00');
	return new Intl.DateTimeFormat('en-US', {
		month: 'short',
		day: 'numeric',
		year: 'numeric'
	}).format(date);
}
