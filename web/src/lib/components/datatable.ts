import type { Component } from 'svelte';

/** How a column is filtered (and which menu control it renders). */
export type FilterType = 'text' | 'enum' | 'date' | 'number';

/**
 * A DataTable column. `key` is the server-side allowlisted identifier (used for
 * sort + `f.<key>` filters). `cell` returns the plain-text display value (no
 * HTML — rendered as text). `enum` columns render their value as a pill.
 */
export interface Column<T> {
	key: string;
	label: string;
	sortable?: boolean;
	filter?: FilterType;
	values?: string[]; // enum options
	cell?: (row: T) => string;
}

/**
 * A table action. `bulk: true` actions appear in the selection bar and receive
 * the selected rows; non-bulk actions are reserved for future per-row menus.
 */
export interface RowAction<T> {
	label: string;
	icon?: Component;
	run: (rows: T[]) => void | Promise<void>;
	danger?: boolean;
	bulk?: boolean;
}
