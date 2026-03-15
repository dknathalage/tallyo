export interface PaginationParams {
	page?: number;
	limit?: number;
}

export interface PaginatedResult<T> {
	data: T[];
	total: number;
	page: number;
	limit: number;
	totalPages: number;
	hasNextPage: boolean;
	hasPrevPage: boolean;
}

export function paginate<T>(
	allItems: T[],
	params?: PaginationParams
): PaginatedResult<T> {
	const page = Math.max(1, params?.page ?? 1);
	const limit = Math.min(Math.max(1, params?.limit ?? 50), 200);
	const total = allItems.length;
	const totalPages = Math.ceil(total / limit) || 1;
	const offset = (page - 1) * limit;
	const data = allItems.slice(offset, offset + limit);

	return {
		data,
		total,
		page,
		limit,
		totalPages,
		hasNextPage: page < totalPages,
		hasPrevPage: page > 1
	};
}
