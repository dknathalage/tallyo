import { describe, it, expect } from 'vitest';
import { paginate } from './pagination.js';

describe('paginate', () => {
	const items = Array.from({ length: 100 }, (_, i) => i + 1);

	it('returns first page with default params', () => {
		const result = paginate(items);
		expect(result.page).toBe(1);
		expect(result.limit).toBe(50);
		expect(result.total).toBe(100);
		expect(result.totalPages).toBe(2);
		expect(result.data).toHaveLength(50);
		expect(result.data[0]).toBe(1);
		expect(result.data[49]).toBe(50);
	});

	it('returns second page correctly', () => {
		const result = paginate(items, { page: 2, limit: 50 });
		expect(result.page).toBe(2);
		expect(result.data[0]).toBe(51);
		expect(result.data[49]).toBe(100);
	});

	it('hasNextPage is true when not on last page', () => {
		const result = paginate(items, { page: 1, limit: 50 });
		expect(result.hasNextPage).toBe(true);
	});

	it('hasNextPage is false on last page', () => {
		const result = paginate(items, { page: 2, limit: 50 });
		expect(result.hasNextPage).toBe(false);
	});

	it('hasPrevPage is false on first page', () => {
		const result = paginate(items, { page: 1 });
		expect(result.hasPrevPage).toBe(false);
	});

	it('hasPrevPage is true on second page', () => {
		const result = paginate(items, { page: 2, limit: 10 });
		expect(result.hasPrevPage).toBe(true);
	});

	it('handles empty array', () => {
		const result = paginate([]);
		expect(result.total).toBe(0);
		expect(result.data).toHaveLength(0);
		expect(result.totalPages).toBe(1);
		expect(result.hasNextPage).toBe(false);
		expect(result.hasPrevPage).toBe(false);
	});

	it('clamps page to minimum 1 when page <= 0', () => {
		const result = paginate(items, { page: 0 });
		expect(result.page).toBe(1);
	});

	it('clamps page to minimum 1 when page is negative', () => {
		const result = paginate(items, { page: -5 });
		expect(result.page).toBe(1);
	});

	it('clamps limit to minimum 1', () => {
		const result = paginate([1, 2, 3], { limit: 0 });
		expect(result.limit).toBe(1);
	});

	it('clamps limit to maximum 200', () => {
		const result = paginate(items, { limit: 999 });
		expect(result.limit).toBe(200);
	});

	it('returns all items when limit > total', () => {
		const small = [1, 2, 3];
		const result = paginate(small, { limit: 10 });
		expect(result.data).toEqual([1, 2, 3]);
		expect(result.totalPages).toBe(1);
	});

	it('uses no params when params is undefined', () => {
		const result = paginate(items, undefined);
		expect(result.page).toBe(1);
		expect(result.limit).toBe(50);
	});

	it('handles limit that does not divide evenly', () => {
		const result = paginate(Array.from({ length: 25 }, (_, i) => i), { limit: 10 });
		expect(result.totalPages).toBe(3);
		expect(result.data).toHaveLength(10);
	});

	it('returns partial last page', () => {
		const result = paginate(Array.from({ length: 25 }, (_, i) => i), { page: 3, limit: 10 });
		expect(result.data).toHaveLength(5);
	});

	it('returns single item page', () => {
		const result = paginate([42], { page: 1, limit: 1 });
		expect(result.data).toEqual([42]);
		expect(result.totalPages).toBe(1);
		expect(result.hasNextPage).toBe(false);
	});

	it('page beyond end returns empty data', () => {
		const result = paginate([1, 2, 3], { page: 99, limit: 10 });
		expect(result.data).toHaveLength(0);
	});
});
