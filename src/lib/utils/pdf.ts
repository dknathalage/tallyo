import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { Invoice, LineItem, Client } from '$lib/types';

export function exportInvoicePdf(invoice: Invoice, lineItems: LineItem[], client: Client): void {
	const doc = new jsPDF();
	const pageWidth = doc.internal.pageSize.getWidth();
	let y = 20;

	// Header
	doc.setFontSize(28);
	doc.setTextColor(37, 99, 235); // primary-600 blue
	doc.text('INVOICE', 14, y);

	doc.setFontSize(12);
	doc.setTextColor(107, 114, 128); // gray-500
	doc.text(invoice.invoice_number, pageWidth - 14, y, { align: 'right' });

	y += 16;

	// Divider
	doc.setDrawColor(229, 231, 235);
	doc.setLineWidth(0.5);
	doc.line(14, y, pageWidth - 14, y);

	y += 12;

	// Bill To section
	doc.setFontSize(9);
	doc.setTextColor(107, 114, 128);
	doc.text('BILL TO', 14, y);

	doc.setFontSize(9);
	doc.text('INVOICE DETAILS', pageWidth / 2 + 10, y);

	y += 6;

	doc.setFontSize(11);
	doc.setTextColor(17, 24, 39); // gray-900
	doc.text(client.name, 14, y);

	// Invoice details on the right
	doc.setFontSize(10);
	const detailsX = pageWidth / 2 + 10;
	const detailsValueX = pageWidth - 14;

	doc.setTextColor(107, 114, 128);
	doc.text('Invoice Number:', detailsX, y);
	doc.setTextColor(17, 24, 39);
	doc.text(invoice.invoice_number, detailsValueX, y, { align: 'right' });

	y += 6;

	if (client.email) {
		doc.setFontSize(10);
		doc.setTextColor(107, 114, 128);
		doc.text(client.email, 14, y);
	}

	doc.setTextColor(107, 114, 128);
	doc.text('Date:', detailsX, y);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfDate(invoice.date), detailsValueX, y, { align: 'right' });

	y += 6;

	if (client.phone) {
		doc.setFontSize(10);
		doc.setTextColor(107, 114, 128);
		doc.text(client.phone, 14, y);
	}

	doc.setTextColor(107, 114, 128);
	doc.text('Due Date:', detailsX, y);
	doc.setTextColor(17, 24, 39);
	doc.text(formatPdfDate(invoice.due_date), detailsValueX, y, { align: 'right' });

	y += 6;

	if (client.address) {
		doc.setFontSize(10);
		doc.setTextColor(107, 114, 128);
		const addressLines = client.address.split('\n');
		for (const line of addressLines) {
			doc.text(line, 14, y);
			y += 5;
		}
	}

	doc.setTextColor(107, 114, 128);
	doc.text('Status:', detailsX, y);
	doc.setTextColor(17, 24, 39);
	doc.text(invoice.status.charAt(0).toUpperCase() + invoice.status.slice(1), detailsValueX, y, { align: 'right' });

	y += 12;

	// Line items table
	const tableBody = lineItems.map((item) => [
		item.description,
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

	// Save
	doc.save(`invoice-${invoice.invoice_number}.pdf`);
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
