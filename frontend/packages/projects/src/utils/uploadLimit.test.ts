import { describe, expect, it } from 'vitest';
import { MAX_UPLOAD_BYTES, precheckUploads, uploadLimitLabel } from './uploadLimit';

// Only name/size/type are read, so a real 200 MB File is not needed (and would
// allocate the bytes).
const file = (name: string, size: number): File => ({ name, size, type: 'application/octet-stream' }) as unknown as File;

describe('precheckUploads', () => {
    it('accepts a file exactly at the limit', () => {
        const { accepted, rejected } = precheckUploads([file('edge.bin', MAX_UPLOAD_BYTES)]);
        expect(rejected).toEqual([]);
        expect(accepted.map((f) => f.name)).toEqual(['edge.bin']);
    });

    it('rejects a file one byte over, and says by how much', () => {
        const { accepted, rejected } = precheckUploads([file('over.bin', MAX_UPLOAD_BYTES + 1)]);
        expect(accepted).toEqual([]);
        expect(rejected).toHaveLength(1);
        expect(rejected[0].file.name).toBe('over.bin');
        expect(rejected[0].size).toBe(MAX_UPLOAD_BYTES + 1);
        expect(rejected[0].over).toBe(1);
    });

    it('keeps the acceptable files when only some are over', () => {
        const { accepted, rejected } = precheckUploads([
            file('small.txt', 10),
            file('huge.bin', MAX_UPLOAD_BYTES * 2),
            file('edge.bin', MAX_UPLOAD_BYTES),
        ]);
        expect(accepted.map((f) => f.name)).toEqual(['small.txt', 'edge.bin']);
        expect(rejected.map((r) => r.file.name)).toEqual(['huge.bin']);
    });

    it('shows the limit as a size a person reads', () => {
        expect(uploadLimitLabel()).toBe('200 MB');
        expect(MAX_UPLOAD_BYTES).toBe(200 * 1024 * 1024);
    });
});
