import { describe, it, expect, vi, beforeEach } from 'vitest';
import { downloadCsv } from './download.js';

const mockCreateObjectURL = vi.fn().mockReturnValue('blob:mock-url');
const mockRevokeObjectURL = vi.fn();
const mockClick = vi.fn();
const mockCreateElement = vi.fn();

beforeEach(() => {
	vi.clearAllMocks();

	mockCreateElement.mockReturnValue({
		href: '',
		download: '',
		click: mockClick
	});

	Object.defineProperty(globalThis, 'URL', {
		value: { createObjectURL: mockCreateObjectURL, revokeObjectURL: mockRevokeObjectURL },
		writable: true
	});

	Object.defineProperty(globalThis, 'document', {
		value: { createElement: mockCreateElement },
		writable: true
	});

	Object.defineProperty(globalThis, 'Blob', {
		value: class MockBlob {
			constructor(public parts: BlobPart[], public options: BlobPropertyBag) {}
		},
		writable: true
	});
});

describe('downloadCsv', () => {
	it('creates a blob with csv content', () => {
		downloadCsv('a,b\n1,2', 'test.csv');
		expect(mockCreateObjectURL).toHaveBeenCalledWith(expect.any(Object));
	});

	it('sets the download filename on the anchor', () => {
		const anchor = { href: '', download: '', click: mockClick };
		mockCreateElement.mockReturnValue(anchor);
		downloadCsv('a,b', 'my-export.csv');
		expect(anchor.download).toBe('my-export.csv');
	});

	it('sets the href to the object URL', () => {
		const anchor = { href: '', download: '', click: mockClick };
		mockCreateElement.mockReturnValue(anchor);
		mockCreateObjectURL.mockReturnValue('blob:some-url');
		downloadCsv('data', 'file.csv');
		expect(anchor.href).toBe('blob:some-url');
	});

	it('clicks the anchor element', () => {
		downloadCsv('data', 'file.csv');
		expect(mockClick).toHaveBeenCalled();
	});

	it('revokes the object URL after clicking', () => {
		mockCreateObjectURL.mockReturnValue('blob:temp-url');
		downloadCsv('data', 'file.csv');
		expect(mockRevokeObjectURL).toHaveBeenCalledWith('blob:temp-url');
	});

	it('creates an anchor element', () => {
		downloadCsv('data', 'file.csv');
		expect(mockCreateElement).toHaveBeenCalledWith('a');
	});
});
