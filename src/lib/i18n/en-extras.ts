import type { Messages } from './types.js';

type Extras = Pick<
	Messages,
	'pwa' | 'pdf' | 'shortcuts' | 'quickAdd' | 'recurring' | 'fileGate'
>;

export const enExtras: Extras = {
	pwa: {
		offlineReady: 'Ready to work offline',
		newVersionAvailable: 'New version available',
		reload: 'Reload',
		dismiss: 'Dismiss'
	},
	pdf: {
		invoice: 'INVOICE',
		estimate: 'ESTIMATE',
		serviceFor: 'SERVICE FOR',
		billTo: 'BILL TO',
		invoiceNumber: 'Invoice #:',
		estimateNumber: 'Estimate #:',
		date: 'Date:',
		due: 'Due:',
		validUntil: 'Valid Until:',
		status: 'Status:',
		description: 'Description',
		quantity: 'Quantity',
		rate: 'Rate',
		amount: 'Amount',
		subtotal: 'Subtotal:',
		tax: 'Tax ({rate}%):',
		total: 'Total:',
		notes: 'NOTES',
		thankYou: 'Thank you for your business'
	},
	shortcuts: {
		title: 'Keyboard Shortcuts',
		newItem: 'New invoice / estimate / client (on list pages)',
		newInvoice: 'New invoice',
		newEstimate: 'New estimate',
		newClient: 'New client',
		focusSearch: 'Focus search input',
		closeModal: 'Close modal / dialog',
		showHelp: 'Show this help'
	},
	quickAdd: {
		label: 'Quick add',
		newInvoice: 'New Invoice',
		newEstimate: 'New Estimate',
		newClient: 'New Client'
	},
	recurring: {
		title: 'Recurring Templates',
		newTemplate: 'New Template',
		editTemplate: 'Edit Template',
		templateName: 'Template Name',
		frequency: 'Frequency',
		nextDue: 'Next Due',
		clientName: 'Client',
		taxRate: 'Tax Rate (%)',
		notes: 'Notes',
		lineItems: 'Line Items',
		isActive: 'Active',
		weekly: 'Weekly',
		monthly: 'Monthly',
		quarterly: 'Quarterly',
		createTemplate: 'Create Template',
		updateTemplate: 'Update Template',
		deleteTemplate: 'Delete Template',
		deleteConfirmTitle: 'Delete Template',
		deleteConfirmMessage:
			'Are you sure you want to delete this recurring template? This action cannot be undone.',
		noTemplates: 'No recurring templates',
		noTemplatesMessage: 'Create a recurring template to automate invoice generation.',
		notFound: 'Template not found.',
		backToTemplates: 'Back to templates',
		createFromTemplate: 'Create Invoice from Template',
		saveAsRecurring: 'Save as Recurring',
		saveAsRecurringTitle: 'Save as Recurring Template',
		templateNamePlaceholder: 'e.g. Monthly Retainer',
		dueNoticeTitle: 'Recurring Templates Due',
		dueNoticeMessage:
			'{count} recurring template{plural} {verb} due. Review and create invoices.',
		viewDue: 'View Due Templates',
		inactive: 'Inactive',
		active: 'Active',
		deactivate: 'Deactivate',
		activate: 'Activate',
		created: 'Invoice created from template.',
		namePlaceholder: 'Template name'
	},
	fileGate: {
		appName: 'Invoice Manager',
		appDescription:
			'A local-first invoice management tool. Your data stays on your device, stored in a SQLite database file you control.',
		reconnectTo: 'Reconnect to {name}',
		createNewDatabase: 'Create New Database',
		openExistingDatabase: 'Open Existing Database',
		dataStaysOnDevice: 'Your data stays on your device.'
	}
};
