import { describe, expect, it } from 'vitest';
import type { DiffFile } from '../../../client';
import { filterFiles, firstReviewable, parsePatch, RowKind, viewedSignature } from './diffModel';

const PATCH = [
    'diff --git a/a.ts b/a.ts',
    'index 1..2 100644',
    '--- a/a.ts',
    '+++ b/a.ts',
    '@@ -3,4 +3,5 @@ fn()',
    ' keep',
    '-gone',
    '+new1',
    '+new2',
    ' tail',
    '\\ No newline at end of file',
    '@@ -20 +21 @@',
    '-x',
    '+y',
    '',
].join('\n');

describe('parsePatch', () => {
    it('drops the file header and numbers both sides', () => {
        const p = parsePatch(PATCH);
        expect(p.text).toEqual(['@@ -3,4 +3,5 @@ fn()', 'keep', 'gone', 'new1', 'new2', 'tail', '\\ No newline at end of file', '@@ -20 +21 @@', 'x', 'y']);
        expect([...p.kind]).toEqual([RowKind.Hunk, RowKind.Ctx, RowKind.Del, RowKind.Add, RowKind.Add, RowKind.Ctx, RowKind.Note, RowKind.Hunk, RowKind.Del, RowKind.Add]);
        expect([...p.oldNo]).toEqual([0, 3, 4, 0, 0, 5, 0, 0, 20, 0]);
        expect([...p.newNo]).toEqual([0, 3, 0, 4, 5, 6, 0, 0, 0, 21]);
    });

    it('reads a 50k-line patch quickly', () => {
        const big = ['@@ -1,50000 +1,50000 @@', ...Array.from({ length: 50000 }, (_, i) => (i % 3 ? ` line ${i}` : `+line ${i}`))].join('\n');
        const t0 = performance.now();
        const p = parsePatch(big);
        expect(p.text).toHaveLength(50001);
        expect(performance.now() - t0).toBeLessThan(500);
    });

    it('is empty for a patch with no hunks', () => {
        expect(parsePatch('').text).toEqual([]);
        expect(parsePatch('diff --git a/x b/x\nBinary files differ\n').text).toEqual([]);
    });
});

const f = (path: string, extra: Partial<DiffFile> = {}): DiffFile => ({ path, status: 'modified', additions: 1, deletions: 1, ...extra });

describe('filterFiles', () => {
    const files = [f('src/a.ts'), f('package-lock.json', { generated: true }), f('img.png', { binary: true }), f('src/new.ts', { old_path: 'src/old.ts', status: 'renamed' })];
    const none = () => false;
    it('filters by path, including a rename source', () => {
        expect(filterFiles(files, { query: 'SRC/', hideGenerated: false, unviewedOnly: false }, none)).toEqual([0, 3]);
        expect(filterFiles(files, { query: 'old', hideGenerated: false, unviewedOnly: false }, none)).toEqual([3]);
    });
    it('hides generated and binary files', () => {
        expect(filterFiles(files, { query: '', hideGenerated: true, unviewedOnly: false }, none)).toEqual([0, 3]);
    });
    it('keeps only the unviewed', () => {
        expect(filterFiles(files, { query: '', hideGenerated: false, unviewedOnly: true }, (x) => x.path === 'src/a.ts')).toEqual([1, 2, 3]);
    });
    it('opens on the first file worth reading', () => {
        expect(firstReviewable(files, [1, 2, 0])).toBe(0);
        expect(firstReviewable(files, [1, 2])).toBe(1);
        expect(firstReviewable(files, [])).toBe(-1);
    });
    it('ties the viewed mark to the file’s change', () => {
        expect(viewedSignature(f('a'))).not.toBe(viewedSignature(f('a', { additions: 2 })));
    });
});
