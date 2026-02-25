import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../connection.svelte.js', () => ({
	query: vi.fn(),
	execute: vi.fn(),
	save: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('../audit.js', () => ({
	logAudit: vi.fn(),
	computeChanges: vi.fn().mockReturnValue({})
}));

import { getBusinessProfile, saveBusinessProfile, buildBusinessSnapshot } from './business-profile.js';
import { query, execute, save } from '../connection.svelte.js';

const mockQuery = vi.mocked(query);
const mockExecute = vi.mocked(execute);
const mockSave = vi.mocked(save);

beforeEach(() => {
	vi.clearAllMocks();
});

describe('getBusinessProfile', () => {
	it('returns profile when exists', () => {
		const profile = { id: 1, name: 'My Business', email: 'biz@test.com' };
		mockQuery.mockReturnValue([profile]);

		expect(getBusinessProfile()).toEqual(profile);
		expect(mockQuery).toHaveBeenCalledWith('SELECT * FROM business_profile WHERE id = 1');
	});

	it('returns null when no profile exists', () => {
		mockQuery.mockReturnValue([]);

		expect(getBusinessProfile()).toBeNull();
	});
});

describe('saveBusinessProfile', () => {
	it('creates new profile with INSERT OR REPLACE', async () => {
		mockQuery.mockReturnValue([]);

		await saveBusinessProfile({ name: 'My Biz', email: 'biz@test.com' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT OR REPLACE INTO business_profile'),
			expect.arrayContaining(['My Biz', 'biz@test.com'])
		);
		expect(mockSave).toHaveBeenCalled();
	});

	it('preserves existing uuid on update', async () => {
		mockQuery.mockReturnValue([{ id: 1, uuid: 'existing-uuid', name: 'Old' }]);

		await saveBusinessProfile({ name: 'Updated' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT OR REPLACE'),
			expect.arrayContaining(['existing-uuid', 'Updated'])
		);
	});

	it('defaults optional fields', async () => {
		mockQuery.mockReturnValue([]);

		await saveBusinessProfile({ name: 'Biz' });

		expect(mockExecute).toHaveBeenCalledWith(
			expect.stringContaining('INSERT OR REPLACE'),
			expect.arrayContaining(['Biz', '', '', '', '', '{}'])
		);
	});
});

describe('buildBusinessSnapshot', () => {
	it('returns empty snapshot when no profile', () => {
		mockQuery.mockReturnValue([]);

		const snapshot = buildBusinessSnapshot();

		expect(snapshot).toEqual({
			name: '',
			email: '',
			phone: '',
			address: '',
			metadata: {}
		});
	});

	it('builds snapshot from profile with metadata', () => {
		mockQuery.mockReturnValue([{
			id: 1,
			uuid: 'test',
			name: 'My Biz',
			email: 'biz@test.com',
			phone: '555-0100',
			address: '123 Main St',
			logo: 'data:image/png;base64,abc',
			metadata: '{"ABN":"12345"}'
		}]);

		const snapshot = buildBusinessSnapshot();

		expect(snapshot).toEqual({
			name: 'My Biz',
			email: 'biz@test.com',
			phone: '555-0100',
			address: '123 Main St',
			logo: 'data:image/png;base64,abc',
			metadata: { ABN: '12345' }
		});
	});
});
