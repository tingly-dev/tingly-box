// Remembers the slice grid/frame for a given source image, so reopening the
// same sheet continues from where the last session left off instead of
// resetting to the default 3x3 grid — the frame drag and gutter nudge are the
// tedious part of this dialog, not something worth redoing on every reopen.
// Background cleanup and tolerance are left out on purpose: those are
// auto-detected per image already (see analyzeSheetBackground), so there is
// nothing to "continue" there.

import type { CropRect } from './imageSlice';

export interface SliceParams {
    rows: number;
    cols: number;
    crop: CropRect;
    gutter: number;
    exportSize: number | null;
    frameDelay: number;
}

const STORAGE_KEY = 'tingly.imageSliceParams.v1';
const MAX_ENTRIES = 50;

// FNV-1a over the image source (a data URL or provider URL) — cheap, stable,
// and a hash collision just means an unrelated image inherits a grid, never a
// crash or lost data.
const hashSrc = (src: string): string => {
    let hash = 0x811c9dc5;
    for (let i = 0; i < src.length; i += 1) {
        hash ^= src.charCodeAt(i);
        hash = Math.imul(hash, 0x01000193);
    }
    return (hash >>> 0).toString(16);
};

type StoredEntry = SliceParams & { savedAt: number };

const readAll = (): Record<string, StoredEntry> => {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return {};
        const parsed = JSON.parse(raw);
        return parsed && typeof parsed === 'object' ? parsed : {};
    } catch {
        return {};
    }
};

/** The grid/frame last saved for this exact image, or null if never saved. */
export const loadSliceParams = (src: string): SliceParams | null => {
    const entry = readAll()[hashSrc(src)];
    if (!entry) return null;
    const { savedAt, ...params } = entry;
    return params;
};

/** Saves (or replaces) the grid/frame for this image, evicting the oldest entries past the cap. */
export const saveSliceParams = (src: string, params: SliceParams): void => {
    try {
        const all = readAll();
        all[hashSrc(src)] = { ...params, savedAt: Date.now() };
        const keys = Object.keys(all);
        if (keys.length > MAX_ENTRIES) {
            keys
                .sort((a, b) => all[a].savedAt - all[b].savedAt)
                .slice(0, keys.length - MAX_ENTRIES)
                .forEach((key) => { delete all[key]; });
        }
        localStorage.setItem(STORAGE_KEY, JSON.stringify(all));
    } catch {
        // Best-effort: a full or disabled localStorage just means no memory.
    }
};
