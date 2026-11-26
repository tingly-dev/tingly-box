/**
 * Pure helpers for the Library panel: the name a person reads, search
 * matching, and ordering. The API returns stored names as they are on disk;
 * the readable name, search and sort all happen here in the browser.
 */

/** An upload is stored as uploads/<12 hex id>-<original name>; show the original name. */
export const displayName = (path: string): string => {
    const base = path.split('/').pop() || path;
    return /^uploads\/[0-9a-f]{12}-[^/]+$/.test(path) ? base.slice(13) : base;
};

/** How the files of every open folder are ordered: by modified time or name, each way. */
export type LibrarySort = 'time-desc' | 'time-asc' | 'name-asc' | 'name-desc';

export const LIBRARY_SORTS: LibrarySort[] = ['time-desc', 'time-asc', 'name-asc', 'name-desc'];

interface SortableEntry {
    name: string;
    path: string;
    kind: 'dir' | 'file';
    updated_at: string;
}

/**
 * Case-insensitive substring match on the name a person sees. toLowerCase
 * folds Latin letters; CJK names match as typed.
 */
export const matchesQuery = (name: string, query: string): boolean => {
    const q = query.trim().toLowerCase();
    return q === '' || name.toLowerCase().includes(q);
};

/**
 * Folders first, then files, in the chosen order. Folders stay by name
 * unless names are being ordered (a folder's mtime moves whenever anything
 * inside it does, so it is a poor key); the same name falls back to path, so
 * the result is stable.
 */
export const sortLibraryEntries = <N extends SortableEntry>(entries: N[], sort: LibrarySort): N[] => {
    const [by, dir] = sort.split('-');
    const desc = dir === 'desc';
    const byName = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);
    return [...entries].sort((a, b) => {
        if ((a.kind === 'dir') !== (b.kind === 'dir')) return a.kind === 'dir' ? -1 : 1;
        if (a.kind === 'dir' && by !== 'name') {
            if (a.name !== b.name) return byName(a.name, b.name);
        } else if (by === 'name') {
            if (a.name !== b.name) return byName(a.name, b.name) * (desc ? -1 : 1);
        } else {
            const at = Date.parse(a.updated_at);
            const bt = Date.parse(b.updated_at);
            if (at !== bt) return Math.sign(at - bt) * (desc ? -1 : 1);
        }
        return byName(a.path, b.path);
    });
};
