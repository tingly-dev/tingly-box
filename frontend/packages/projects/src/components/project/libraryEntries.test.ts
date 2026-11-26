import { expect, it } from 'vitest';
import { displayName, matchesQuery, sortLibraryEntries } from './libraryEntries';

it('shows an upload under the name it was given, stripping exactly one stored id', () => {
    expect(displayName('uploads/0123456789ab-report final.pdf')).toBe('report final.pdf');
    expect(displayName('uploads/0123456789ab-a-b.txt')).toBe('a-b.txt');
    expect(displayName('uploads/notes.md')).toBe('notes.md');
    // Stored as <id>-<original name>; an original that itself looks stored keeps its own prefix.
    expect(displayName('uploads/0123456789ab-fedcba987654-looks-stored.txt')).toBe('fedcba987654-looks-stored.txt');
    // Only files directly in uploads/ are rewritten; elsewhere the name is real.
    expect(displayName('outputs/0123456789ab-x.txt')).toBe('0123456789ab-x.txt');
    expect(displayName('uploads/sub/0123456789ab-x.txt')).toBe('0123456789ab-x.txt');
    expect(displayName('uploads/0123456789AB-x.txt')).toBe('0123456789AB-x.txt');
    expect(displayName('notes.md')).toBe('notes.md');
});

it('matches a name case-insensitively, including CJK', () => {
    expect(matchesQuery('report final.pdf', '')).toBe(true);
    expect(matchesQuery('report final.pdf', '   ')).toBe(true);
    expect(matchesQuery('report final.pdf', 'Report')).toBe(true);
    expect(matchesQuery('REPORT.PDF', 'report')).toBe(true);
    expect(matchesQuery('季度报告.md', '报告')).toBe(true);
    expect(matchesQuery('季度报告.md', '季度')).toBe(true);
    expect(matchesQuery('notes.md', 'xyz')).toBe(false);
    expect(matchesQuery('季度报告.md', '年度')).toBe(false);
});

const entry = (path: string, kind: 'dir' | 'file', at: string, name = path.split('/').pop()!) => ({ path, name, kind, updated_at: at });
const order = (entries: ReturnType<typeof entry>[], sort: Parameters<typeof sortLibraryEntries>[1]) => sortLibraryEntries(entries, sort).map((e) => e.name);

it('orders folders first, then files by time or name, each way', () => {
    const entries = [
        entry('uploads/b.txt', 'file', '2026-01-01T00:00:03Z'),
        entry('uploads/a.txt', 'file', '2026-01-01T00:00:01Z'),
        entry('uploads/c.txt', 'file', '2026-01-01T00:00:02Z'),
        entry('uploads/z-dir', 'dir', '2026-01-01T00:00:00Z'),
        entry('uploads/a-dir', 'dir', '2026-01-01T00:00:09Z'),
    ];
    expect(order(entries, 'time-desc')).toEqual(['a-dir', 'z-dir', 'b.txt', 'c.txt', 'a.txt']);
    expect(order(entries, 'time-asc')).toEqual(['a-dir', 'z-dir', 'a.txt', 'c.txt', 'b.txt']);
    expect(order(entries, 'name-asc')).toEqual(['a-dir', 'z-dir', 'a.txt', 'b.txt', 'c.txt']);
    // Folders follow the direction only when names are the key.
    expect(order(entries, 'name-desc')).toEqual(['z-dir', 'a-dir', 'c.txt', 'b.txt', 'a.txt']);
});

it('falls back to the path so equal keys keep a stable order', () => {
    const entries = [
        entry('uploads/0123456789ab-x.txt', 'file', '2026-01-01T00:00:00Z', 'x.txt'),
        entry('uploads/abcdefabcdef-x.txt', 'file', '2026-01-01T00:00:00Z', 'x.txt'),
    ];
    expect(sortLibraryEntries(entries, 'time-desc').map((e) => e.path)).toEqual(['uploads/0123456789ab-x.txt', 'uploads/abcdefabcdef-x.txt']);
});
