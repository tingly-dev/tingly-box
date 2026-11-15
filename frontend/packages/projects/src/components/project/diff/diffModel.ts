import type { DiffFile } from '../../../client';

export const RowKind = { Hunk: 0, Add: 1, Del: 2, Ctx: 3, Note: 4 } as const;
export type RowKind = (typeof RowKind)[keyof typeof RowKind];

/** A file's patch read into parallel arrays: one entry per displayed line, so a
 * 100k-line patch costs a few typed arrays and strings, not 100k objects. */
export interface ParsedPatch {
    /** The text of each row; add/del/context rows lose their leading +, - or space. */
    text: string[];
    kind: Uint8Array;
    oldNo: Int32Array;
    newNo: Int32Array;
    /** Length of the longest row, to size the horizontal scroll. */
    maxLen: number;
}

const HUNK = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/;

/** Reads one file's `git diff` output. The header lines before the first hunk
 * (diff --git, index, ---, +++, rename from…) are dropped: the viewer's own
 * header already says which file it is. */
export function parsePatch(patch: string): ParsedPatch {
    const lines = patch.split('\n');
    if (lines[lines.length - 1] === '') lines.pop();
    const text: string[] = [];
    const kind = new Uint8Array(lines.length);
    const oldNo = new Int32Array(lines.length);
    const newNo = new Int32Array(lines.length);
    let n = 0;
    let o = 0;
    let w = 0;
    let inHunk = false;
    let maxLen = 0;
    for (const line of lines) {
        const m = HUNK.exec(line);
        let k: RowKind;
        let t = line;
        if (m) {
            inHunk = true;
            o = Number(m[1]);
            w = Number(m[2]);
            k = RowKind.Hunk;
        } else if (!inHunk) {
            continue;
        } else if (line.startsWith('+')) {
            k = RowKind.Add;
            t = line.slice(1);
            newNo[n] = w++;
        } else if (line.startsWith('-')) {
            k = RowKind.Del;
            t = line.slice(1);
            oldNo[n] = o++;
        } else if (line.startsWith('\\')) {
            k = RowKind.Note;
        } else {
            k = RowKind.Ctx;
            t = line.slice(1);
            oldNo[n] = o++;
            newNo[n] = w++;
        }
        kind[n] = k;
        text.push(t);
        if (t.length > maxLen) maxLen = t.length;
        n++;
    }
    return { text, kind: kind.subarray(0, n), oldNo: oldNo.subarray(0, n), newNo: newNo.subarray(0, n), maxLen };
}

export interface FileFilter {
    query: string;
    hideGenerated: boolean;
    unviewedOnly: boolean;
}

/** The files that pass the filter, as indexes into `files`. */
export function filterFiles(files: DiffFile[], f: FileFilter, isViewed: (file: DiffFile) => boolean): number[] {
    const q = f.query.trim().toLowerCase();
    const out: number[] = [];
    files.forEach((file, i) => {
        if (q && !file.path.toLowerCase().includes(q) && !(file.old_path ?? '').toLowerCase().includes(q)) return;
        if (f.hideGenerated && (file.generated || file.binary)) return;
        if (f.unviewedOnly && isViewed(file)) return;
        out.push(i);
    });
    return out;
}

/** A file is "viewed" until its change moves on: the mark is tied to its size. */
export function viewedSignature(file: DiffFile): string {
    return `${file.status}:${file.additions}:${file.deletions}:${file.old_path ?? ''}`;
}

export type ViewedMap = Record<string, string>;

const viewedKey = (threadId: string) => `tingly.projects.diffViewed.${threadId}`;

export function loadViewed(threadId: string): ViewedMap {
    try {
        const raw = window.localStorage.getItem(viewedKey(threadId));
        const parsed: unknown = raw ? JSON.parse(raw) : {};
        return parsed && typeof parsed === 'object' ? (parsed as ViewedMap) : {};
    } catch {
        return {};
    }
}

export function saveViewed(threadId: string, viewed: ViewedMap): void {
    try {
        if (Object.keys(viewed).length === 0) window.localStorage.removeItem(viewedKey(threadId));
        else window.localStorage.setItem(viewedKey(threadId), JSON.stringify(viewed));
    } catch {
        // Storage is full or off: the marks last for this view only.
    }
}

/** Changed lines above which a file's patch is not fetched until asked for. */
export const LARGE_FILE_LINES = 4000;

/** The first file worth opening: one that opens by itself, not generated, binary or huge. */
export function firstReviewable(files: DiffFile[], indexes: number[]): number {
    return indexes.find((i) => !files[i].generated && !files[i].binary && files[i].additions + files[i].deletions <= LARGE_FILE_LINES) ?? indexes[0] ?? -1;
}
