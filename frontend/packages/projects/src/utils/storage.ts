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

export function removeLocal(key: string) {
    try {
        window.localStorage.removeItem(PREFIX + key);
    } catch {
        // ignore
    }
}

// sessionStorage holds the same kind of conveniences, but only for the
// current tab's visit (a first-run hint that may still be pending).
export function readSession(key: string): string | null {
    try {
        return window.sessionStorage.getItem(PREFIX + key);
    } catch {
        return null;
    }
}

export function writeSession(key: string, value: string) {
    try {
        window.sessionStorage.setItem(PREFIX + key, value);
    } catch {
        // ignore
    }
}
