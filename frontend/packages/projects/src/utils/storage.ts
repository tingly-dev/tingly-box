// localStorage holds per-browser conveniences only (theme, last model, sidebar
// state). It can throw or be empty in private windows, so every access is
// guarded and callers must work without it.
const PREFIX = 'tingly-project.';

export function readLocal(key: string): string | null {
    try {
        return window.localStorage.getItem(PREFIX + key);
    } catch {
        return null;
    }
}

export function writeLocal(key: string, value: string) {
    try {
        window.localStorage.setItem(PREFIX + key, value);
    } catch {
        // ignore
    }
}
