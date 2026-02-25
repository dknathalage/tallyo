export const CLIENT_COLUMNS = ['uuid', 'name', 'email', 'phone', 'address'] as const;
export const CATALOG_COLUMNS = ['uuid', 'name', 'rate', 'unit', 'category', 'sku'] as const;
export const INVOICE_COLUMNS = [
	'invoice_uuid', 'invoice_number', 'client_name', 'client_email',
	'date', 'due_date', 'tax_rate', 'notes', 'status',
	'line_description', 'line_quantity', 'line_rate', 'line_amount', 'line_sort_order'
] as const;

export const REQUIRED_CLIENT_FIELDS = ['name'] as const;
export const REQUIRED_CATALOG_FIELDS = ['name'] as const;
export const REQUIRED_INVOICE_FIELDS = ['invoice_number', 'client_name', 'date', 'line_description'] as const;
