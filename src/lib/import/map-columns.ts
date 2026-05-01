/**
 * Destination key for a source column. Canonical values: 'name', 'sku', 'unit',
 * 'category', 'rate', 'skip'. Other values represent tier or metadata column names.
 */
export type TargetField = string;

export interface ColumnMappingConfig {
	fieldMap: Record<string, TargetField>;
	tierColumns: Record<string, number>;
	newTierColumns: string[];
	metadataColumns: string[];
}

export interface MappedRow {
	name: string;
	sku: string;
	unit: string;
	category: string;
	rate: number;
	tierRates: Record<number, number>;
	metadata: Record<string, string>;
	_raw: Record<string, string>;
	_errors: string[];
}

const FUZZY_MAP: Record<string, TargetField> = {
	name: 'name',
	'item name': 'name',
	'item_name': 'name',
	'service name': 'name',
	'support item name': 'name',
	description: 'name',
	title: 'name',
	sku: 'sku',
	code: 'sku',
	'item code': 'sku',
	'item_code': 'sku',
	'support item number': 'sku',
	'registration group number': 'category',
	unit: 'unit',
	'unit of measure': 'unit',
	uom: 'unit',
	category: 'category',
	group: 'category',
	'registration group': 'category',
	rate: 'rate',
	price: 'rate',
	amount: 'rate',
	cost: 'rate',
	'unit price': 'rate'
};

// Known unit-of-measure values for smart detection
const KNOWN_UNITS = new Set([
	'h', 'hr', 'hrs', 'hour', 'hours',
	'ea', 'each', 'e',
	'd', 'day', 'days',
	'wk', 'week', 'weeks',
	'mo', 'month', 'months',
	'yr', 'year', 'years',
	'km', 'mi', 'mile', 'miles',
	'kg', 'lb', 'lbs',
	'l', 'litre', 'liter', 'litres', 'liters',
	'unit', 'units',
	'session', 'sessions',
	'visit', 'visits',
	'item', 'items',
	'flat', 'fixed',
	'per item', 'per hour', 'per day', 'per week', 'per month', 'per year',
	'annually', 'monthly', 'weekly', 'daily', 'hourly'
]);

interface ColumnProfile {
	header: string;
	uniquenessRatio: number;
	avgLength: number;
	maxLength: number;
	spaceFraction: number;
	priceFraction: number;
	decimalFraction: number;
	currencyFraction: number;
	numericFraction: number;
	unitMatchFraction: number;
	codeLikeFraction: number;
	cardinality: number;
	nonEmptyCount: number;
}

interface ColumnCounts {
	totalLength: number;
	maxLength: number;
	space: number;
	price: number;
	decimal: number;
	currency: number;
	numeric: number;
	unit: number;
	code: number;
}

function emptyCounts(): ColumnCounts {
	return {
		totalLength: 0,
		maxLength: 0,
		space: 0,
		price: 0,
		decimal: 0,
		currency: 0,
		numeric: 0,
		unit: 0,
		code: 0
	};
}

function tallyValue(v: string, counts: ColumnCounts): void {
	counts.totalLength += v.length;
	if (v.length > counts.maxLength) counts.maxLength = v.length;
	if (/\s/.test(v)) counts.space++;
	if (looksLikePrice(v)) counts.price++;
	const stripped = v.replace(/[$,\s]/g, '');
	if (/\.\d{1,2}/.test(stripped)) counts.decimal++;
	if (/[$]/.test(v)) counts.currency++;
	if (stripped && !isNaN(Number(stripped))) counts.numeric++;
	if (KNOWN_UNITS.has(v.toLowerCase())) counts.unit++;
	// Code-like: alphanumeric with dashes, underscores, dots, slashes; no spaces; 2+ chars
	if (/^[A-Za-z0-9][A-Za-z0-9._\-/]{1,}$/.test(v) && !/\s/.test(v)) counts.code++;
}

function profileColumn(header: string, rows: Record<string, string>[]): ColumnProfile {
	const values = rows.map((r) => (r[header] ?? '').trim());
	const nonEmpty = values.filter((v) => v.length > 0);
	const distinct = new Set(nonEmpty.map((v) => v.toLowerCase()));
	const nonEmptyCount = nonEmpty.length;
	const cardinality = distinct.size;
	const uniquenessRatio = nonEmptyCount > 0 ? cardinality / nonEmptyCount : 0;

	const counts = emptyCounts();
	for (const v of nonEmpty) tallyValue(v, counts);

	const denom = nonEmptyCount > 0 ? nonEmptyCount : 1;
	return {
		header,
		uniquenessRatio,
		avgLength: nonEmptyCount > 0 ? counts.totalLength / denom : 0,
		maxLength: counts.maxLength,
		spaceFraction: nonEmptyCount > 0 ? counts.space / denom : 0,
		priceFraction: nonEmptyCount > 0 ? counts.price / denom : 0,
		decimalFraction: nonEmptyCount > 0 ? counts.decimal / denom : 0,
		currencyFraction: nonEmptyCount > 0 ? counts.currency / denom : 0,
		numericFraction: nonEmptyCount > 0 ? counts.numeric / denom : 0,
		unitMatchFraction: nonEmptyCount > 0 ? counts.unit / denom : 0,
		codeLikeFraction: nonEmptyCount > 0 ? counts.code / denom : 0,
		cardinality,
		nonEmptyCount
	};
}

function scorePriceColumn(p: ColumnProfile): number | null {
	if (p.numericFraction < 0.5) return null;
	if (p.currencyFraction === 0 && p.decimalFraction < 0.1 && p.uniquenessRatio < 0.2) return null;
	let score = 0;
	if (p.currencyFraction > 0.1) score += 3;
	if (p.decimalFraction > 0.3) score += 3;
	if (p.priceFraction > 0.5) score += 1;
	if (p.uniquenessRatio > 0.3) score += 1;
	return score >= 2 ? score : null;
}

function scoreUnitColumn(p: ColumnProfile): number {
	let score = 0;
	if (p.unitMatchFraction > 0.3) score += 5;
	if (p.cardinality <= 15 && p.cardinality >= 1) score += 2;
	if (p.avgLength < 12) score += 1;
	return score;
}

function scoreSkuColumn(p: ColumnProfile): number {
	let score = 0;
	if (p.uniquenessRatio > 0.8) score += 3;
	else if (p.uniquenessRatio > 0.5) score += 1;
	if (p.codeLikeFraction > 0.5) score += 3;
	else if (p.codeLikeFraction > 0.3) score += 1;
	if (p.avgLength >= 3 && p.avgLength <= 30) score += 1;
	if (p.spaceFraction < 0.2) score += 1;
	if (p.spaceFraction > 0.5 && p.avgLength > 20) score -= 2;
	return score;
}

function scoreNameColumn(p: ColumnProfile): number {
	let score = 0;
	if (p.avgLength > 20) score += 3;
	else if (p.avgLength > 10) score += 2;
	else if (p.avgLength > 5) score += 1;
	if (p.spaceFraction > 0.5) score += 2;
	if (p.uniquenessRatio > 0.5) score += 2;
	else if (p.uniquenessRatio > 0.3) score += 1;
	if (p.numericFraction < 0.3) score += 1;
	if (p.maxLength > 30) score += 1;
	return score;
}

function scoreCategoryColumn(p: ColumnProfile): number {
	if (p.nonEmptyCount === 0) return 0;
	let score = 0;
	const cardinalityRatio = p.cardinality / p.nonEmptyCount;
	if (cardinalityRatio < 0.1) score += 3;
	else if (cardinalityRatio < 0.3) score += 2;
	if (p.cardinality >= 2 && p.cardinality <= 50) score += 2;
	if (p.avgLength > 1) score += 1;
	if (p.unitMatchFraction > 0.3) score -= 2;
	return score;
}

function pickBest(
	remaining: Set<string>,
	profiles: Map<string, ColumnProfile>,
	scorer: (p: ColumnProfile) => number,
	threshold: number
): string | null {
	let best: { header: string; score: number } | null = null;
	for (const header of remaining) {
		const p = profiles.get(header);
		if (!p || p.nonEmptyCount === 0) continue;
		const score = scorer(p);
		if (score >= threshold && (!best || score > best.score)) {
			best = { header, score };
		}
	}
	return best ? best.header : null;
}

interface AssignmentState {
	remaining: Set<string>;
	profiles: Map<string, ColumnProfile>;
	assigned: Set<TargetField>;
	fieldMap: Record<string, TargetField>;
	newTiers: string[];
}

function assignPriceColumns(state: AssignmentState): void {
	const { remaining, profiles, assigned, fieldMap, newTiers } = state;
	const priceColumns: { header: string; score: number }[] = [];
	for (const header of remaining) {
		const p = profiles.get(header);
		if (!p || p.nonEmptyCount === 0) continue;
		const score = scorePriceColumn(p);
		if (score !== null) priceColumns.push({ header, score });
	}
	priceColumns.sort((a, b) => b.score - a.score);
	for (const { header } of priceColumns) {
		if (!assigned.has('rate')) {
			fieldMap[header] = 'rate';
			assigned.add('rate');
		} else {
			newTiers.push(header);
		}
		remaining.delete(header);
	}
}

function assignSingleField(
	state: AssignmentState,
	field: TargetField,
	threshold: number,
	scorer: (p: ColumnProfile) => number
): void {
	if (state.assigned.has(field)) return;
	const winner = pickBest(state.remaining, state.profiles, scorer, threshold);
	if (!winner) return;
	state.fieldMap[winner] = field;
	state.assigned.add(field);
	state.remaining.delete(winner);
}

/**
 * Smart field assignment using data profiling heuristics.
 * Analyzes column data for uniqueness, text patterns, currency symbols, and cardinality
 * to automatically assign fields, tiers, and metadata.
 */
function smartAssignFields(
	unmappedHeaders: string[],
	profiles: Map<string, ColumnProfile>,
	alreadyAssigned: Set<TargetField>
): { fieldMap: Record<string, TargetField>; newTiers: string[]; metadata: string[] } {
	const state: AssignmentState = {
		remaining: new Set(unmappedHeaders),
		profiles,
		assigned: new Set(alreadyAssigned),
		fieldMap: {},
		newTiers: []
	};

	assignPriceColumns(state);
	assignSingleField(state, 'unit', 5, scoreUnitColumn);
	assignSingleField(state, 'sku', 4, scoreSkuColumn);
	assignSingleField(state, 'name', 3, scoreNameColumn);
	assignSingleField(state, 'category', 3, scoreCategoryColumn);

	const metadata: string[] = [];
	for (const header of state.remaining) metadata.push(header);

	return { fieldMap: state.fieldMap, newTiers: state.newTiers, metadata };
}

export interface AutoDetectResult {
	fieldMap: Partial<Record<string, TargetField>>;
	suggestedNewTiers: string[];
	suggestedMetadata: string[];
}

export function autoDetectMapping(
	headers: string[],
	sampleRows?: Record<string, string>[]
): AutoDetectResult {
	const fieldMap: Record<string, TargetField> = {};
	const assigned = new Set<TargetField>();

	// First pass: exact/fuzzy matches by header name
	for (const header of headers) {
		const normalized = header.toLowerCase().trim();
		if (FUZZY_MAP[normalized]) {
			fieldMap[header] = FUZZY_MAP[normalized];
			assigned.add(FUZZY_MAP[normalized]);
		}
	}

	// Second pass: smart data-driven assignment for unmapped columns
	if (sampleRows && sampleRows.length > 0) {
		const unmapped = headers.filter((h) => !fieldMap[h]);
		if (unmapped.length > 0) {
			const profiles = new Map<string, ColumnProfile>();
			for (const header of unmapped) {
				profiles.set(header, profileColumn(header, sampleRows));
			}

			const smart = smartAssignFields(unmapped, profiles, assigned);

			for (const [header, field] of Object.entries(smart.fieldMap)) {
				fieldMap[header] = field;
			}

			return {
				fieldMap,
				suggestedNewTiers: smart.newTiers,
				suggestedMetadata: smart.metadata
			};
		}
	}

	return { fieldMap, suggestedNewTiers: [], suggestedMetadata: [] };
}

function looksLikePrice(value: string): boolean {
	const cleaned = value.replace(/[$,\s]/g, '').trim();
	if (!cleaned) return false;
	return /^\d+(\.\d{1,2})?$/.test(cleaned);
}

interface CoreFields {
	name: string;
	sku: string;
	unit: string;
	category: string;
	rate: number;
}

function applyFieldMap(
	row: Record<string, string>,
	fieldMap: ColumnMappingConfig['fieldMap'],
	errors: string[]
): CoreFields {
	const result: CoreFields = { name: '', sku: '', unit: '', category: '', rate: 0 };
	for (const [sourceCol, targetField] of Object.entries(fieldMap)) {
		const value = row[sourceCol] ?? '';
		assignField(result, targetField, value, errors);
	}
	return result;
}

function assignField(
	result: CoreFields,
	targetField: string,
	value: string,
	errors: string[]
): void {
	switch (targetField) {
		case 'name':
			result.name = value.trim();
			return;
		case 'sku':
			result.sku = value.trim();
			return;
		case 'unit':
			result.unit = value.trim();
			return;
		case 'category':
			result.category = value.trim();
			return;
		case 'rate': {
			const parsed = parseRate(value);
			if (parsed !== null) {
				result.rate = parsed;
			} else if (value.trim()) {
				errors.push(`Invalid rate value: "${value}"`);
			}
			return;
		}
		case 'skip':
			return;
		default:
			return;
	}
}

function applyTierColumns(
	row: Record<string, string>,
	tierColumns: ColumnMappingConfig['tierColumns'],
	errors: string[]
): Record<number, number> {
	const tierRates: Record<number, number> = {};
	for (const [sourceCol, tierId] of Object.entries(tierColumns)) {
		const value = row[sourceCol] ?? '';
		const parsed = parseRate(value);
		if (parsed !== null) {
			tierRates[tierId] = parsed;
		} else if (value.trim()) {
			errors.push(`Invalid tier rate for "${sourceCol}": "${value}"`);
		}
	}
	return tierRates;
}

function applyMetadataColumns(
	row: Record<string, string>,
	metadataColumns: ColumnMappingConfig['metadataColumns']
): Record<string, string> {
	const metadata: Record<string, string> = {};
	for (const sourceCol of metadataColumns) {
		const value = row[sourceCol] ?? '';
		if (value.trim()) {
			metadata[sourceCol] = value.trim();
		}
	}
	return metadata;
}

export function applyMapping(rows: Record<string, string>[], config: ColumnMappingConfig): MappedRow[] {
	return rows.map((row) => {
		const errors: string[] = [];
		const core = applyFieldMap(row, config.fieldMap, errors);
		const tierRates = applyTierColumns(row, config.tierColumns, errors);
		const metadata = applyMetadataColumns(row, config.metadataColumns);
		if (!core.name) {
			errors.push('Name is required');
		}
		return { ...core, tierRates, metadata, _raw: row, _errors: errors };
	});
}

function parseRate(value: string): number | null {
	const cleaned = value.replace(/[$,\s]/g, '').trim();
	if (!cleaned) return null;
	const num = Number(cleaned);
	return isNaN(num) ? null : num;
}
