export type TargetField = 'name' | 'sku' | 'unit' | 'category' | 'rate' | 'skip' | string;

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

function profileColumn(header: string, rows: Record<string, string>[]): ColumnProfile {
	const values = rows.map((r) => (r[header] ?? '').trim());
	const nonEmpty = values.filter((v) => v.length > 0);
	const distinct = new Set(nonEmpty.map((v) => v.toLowerCase()));
	const nonEmptyCount = nonEmpty.length;
	const cardinality = distinct.size;
	const uniquenessRatio = nonEmptyCount > 0 ? cardinality / nonEmptyCount : 0;

	let totalLength = 0;
	let maxLength = 0;
	let spaceCount = 0;
	let priceCount = 0;
	let decimalCount = 0;
	let currencyCount = 0;
	let numericCount = 0;
	let unitCount = 0;
	let codeCount = 0;

	for (const v of nonEmpty) {
		totalLength += v.length;
		if (v.length > maxLength) maxLength = v.length;
		if (/\s/.test(v)) spaceCount++;
		if (looksLikePrice(v)) priceCount++;
		if (/\.\d{1,2}/.test(v.replace(/[$,\s]/g, ''))) decimalCount++;
		if (/[$]/.test(v)) currencyCount++;
		const cleaned = v.replace(/[$,\s]/g, '');
		if (cleaned && !isNaN(Number(cleaned))) numericCount++;
		if (KNOWN_UNITS.has(v.toLowerCase())) unitCount++;
		// Code-like: alphanumeric with dashes, underscores, dots, slashes; no spaces; 2+ chars
		if (/^[A-Za-z0-9][A-Za-z0-9._\-/]{1,}$/.test(v) && !/\s/.test(v)) codeCount++;
	}

	return {
		header,
		uniquenessRatio,
		avgLength: nonEmptyCount > 0 ? totalLength / nonEmptyCount : 0,
		maxLength,
		spaceFraction: nonEmptyCount > 0 ? spaceCount / nonEmptyCount : 0,
		priceFraction: nonEmptyCount > 0 ? priceCount / nonEmptyCount : 0,
		decimalFraction: nonEmptyCount > 0 ? decimalCount / nonEmptyCount : 0,
		currencyFraction: nonEmptyCount > 0 ? currencyCount / nonEmptyCount : 0,
		numericFraction: nonEmptyCount > 0 ? numericCount / nonEmptyCount : 0,
		unitMatchFraction: nonEmptyCount > 0 ? unitCount / nonEmptyCount : 0,
		codeLikeFraction: nonEmptyCount > 0 ? codeCount / nonEmptyCount : 0,
		cardinality,
		nonEmptyCount
	};
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
	const fieldMap: Record<string, TargetField> = {};
	const newTiers: string[] = [];
	const metadata: string[] = [];
	const assigned = new Set(alreadyAssigned);
	const remaining = new Set(unmappedHeaders);

	// Phase 1: Identify and assign price/rate columns
	// Uses currency symbols ($), decimal patterns, and high cardinality to distinguish
	// real prices from numeric category codes
	const priceColumns: { header: string; score: number }[] = [];
	for (const header of remaining) {
		const p = profiles.get(header);
		if (!p || p.nonEmptyCount === 0) continue;
		if (p.numericFraction < 0.5) continue;

		let score = 0;
		if (p.currencyFraction > 0.1) score += 3;
		if (p.decimalFraction > 0.3) score += 3;
		if (p.priceFraction > 0.5) score += 1;
		if (p.uniquenessRatio > 0.3) score += 1;
		// Reject low-cardinality integers without decimals/currency (likely category codes)
		if (p.currencyFraction === 0 && p.decimalFraction < 0.1 && p.uniquenessRatio < 0.2) continue;

		if (score >= 2) {
			priceColumns.push({ header, score });
		}
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

	// Phase 2: Unit detection — very specific pattern, check early
	// Looks for columns where values match known unit abbreviations (H, EA, Day, etc.)
	if (!assigned.has('unit')) {
		let best: { header: string; score: number } | null = null;
		for (const header of remaining) {
			const p = profiles.get(header)!;
			if (!p || p.nonEmptyCount === 0) continue;
			let score = 0;
			if (p.unitMatchFraction > 0.3) score += 5;
			if (p.cardinality <= 15 && p.cardinality >= 1) score += 2;
			if (p.avgLength < 12) score += 1;
			if (score >= 5 && (!best || score > best.score)) {
				best = { header, score };
			}
		}
		if (best) {
			fieldMap[best.header] = 'unit';
			assigned.add('unit');
			remaining.delete(best.header);
		}
	}

	// Phase 3: SKU detection — high uniqueness + code-like patterns (alphanumeric with dashes/underscores)
	if (!assigned.has('sku')) {
		let best: { header: string; score: number } | null = null;
		for (const header of remaining) {
			const p = profiles.get(header)!;
			if (!p || p.nonEmptyCount === 0) continue;
			let score = 0;
			if (p.uniquenessRatio > 0.8) score += 3;
			else if (p.uniquenessRatio > 0.5) score += 1;
			if (p.codeLikeFraction > 0.5) score += 3;
			else if (p.codeLikeFraction > 0.3) score += 1;
			if (p.avgLength >= 3 && p.avgLength <= 30) score += 1;
			if (p.spaceFraction < 0.2) score += 1;
			// Penalize descriptive text
			if (p.spaceFraction > 0.5 && p.avgLength > 20) score -= 2;
			if (score >= 4 && (!best || score > best.score)) {
				best = { header, score };
			}
		}
		if (best) {
			fieldMap[best.header] = 'sku';
			assigned.add('sku');
			remaining.delete(best.header);
		}
	}

	// Phase 4: Name detection — long descriptive text, high uniqueness, contains spaces
	if (!assigned.has('name')) {
		let best: { header: string; score: number } | null = null;
		for (const header of remaining) {
			const p = profiles.get(header)!;
			if (!p || p.nonEmptyCount === 0) continue;
			let score = 0;
			if (p.avgLength > 20) score += 3;
			else if (p.avgLength > 10) score += 2;
			else if (p.avgLength > 5) score += 1;
			if (p.spaceFraction > 0.5) score += 2;
			if (p.uniquenessRatio > 0.5) score += 2;
			else if (p.uniquenessRatio > 0.3) score += 1;
			if (p.numericFraction < 0.3) score += 1;
			if (p.maxLength > 30) score += 1;
			if (score >= 3 && (!best || score > best.score)) {
				best = { header, score };
			}
		}
		if (best) {
			fieldMap[best.header] = 'name';
			assigned.add('name');
			remaining.delete(best.header);
		}
	}

	// Phase 5: Category detection — low cardinality grouping column
	if (!assigned.has('category')) {
		let best: { header: string; score: number } | null = null;
		for (const header of remaining) {
			const p = profiles.get(header)!;
			if (!p || p.nonEmptyCount === 0) continue;
			let score = 0;
			const cardinalityRatio = p.cardinality / p.nonEmptyCount;
			if (cardinalityRatio < 0.1) score += 3;
			else if (cardinalityRatio < 0.3) score += 2;
			if (p.cardinality >= 2 && p.cardinality <= 50) score += 2;
			if (p.avgLength > 1) score += 1;
			// Don't confuse with unit columns
			if (p.unitMatchFraction > 0.3) score -= 2;
			if (score >= 3 && (!best || score > best.score)) {
				best = { header, score };
			}
		}
		if (best) {
			fieldMap[best.header] = 'category';
			assigned.add('category');
			remaining.delete(best.header);
		}
	}

	// Phase 6: Everything remaining → metadata
	for (const header of remaining) {
		metadata.push(header);
	}

	return { fieldMap, newTiers, metadata };
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

export function applyMapping(rows: Record<string, string>[], config: ColumnMappingConfig): MappedRow[] {
	return rows.map((row) => {
		const errors: string[] = [];
		let name = '';
		let sku = '';
		let unit = '';
		let category = '';
		let rate = 0;
		const tierRates: Record<number, number> = {};
		const metadata: Record<string, string> = {};

		for (const [sourceCol, targetField] of Object.entries(config.fieldMap)) {
			const value = row[sourceCol] ?? '';
			switch (targetField) {
				case 'name':
					name = value.trim();
					break;
				case 'sku':
					sku = value.trim();
					break;
				case 'unit':
					unit = value.trim();
					break;
				case 'category':
					category = value.trim();
					break;
				case 'rate': {
					const parsed = parseRate(value);
					if (parsed !== null) {
						rate = parsed;
					} else if (value.trim()) {
						errors.push(`Invalid rate value: "${value}"`);
					}
					break;
				}
				case 'skip':
					break;
			}
		}

		for (const [sourceCol, tierId] of Object.entries(config.tierColumns)) {
			const value = row[sourceCol] ?? '';
			const parsed = parseRate(value);
			if (parsed !== null) {
				tierRates[tierId] = parsed;
			} else if (value.trim()) {
				errors.push(`Invalid tier rate for "${sourceCol}": "${value}"`);
			}
		}

		for (const sourceCol of config.metadataColumns) {
			const value = row[sourceCol] ?? '';
			if (value.trim()) {
				metadata[sourceCol] = value.trim();
			}
		}

		if (!name) {
			errors.push('Name is required');
		}

		return { name, sku, unit, category, rate, tierRates, metadata, _raw: row, _errors: errors };
	});
}

function parseRate(value: string): number | null {
	const cleaned = value.replace(/[$,\s]/g, '').trim();
	if (!cleaned) return null;
	const num = Number(cleaned);
	return isNaN(num) ? null : num;
}
