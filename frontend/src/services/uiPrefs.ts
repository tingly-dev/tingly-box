// UI preferences shared by every UI surface. The browser tab and the Wails
// desktop window are different origins with separate localStorage, so a
// preference kept only there (hidden agents, Quick Start progress) differs
// between them. The keys listed in SYNCED are mirrored to the server
// (GET/PATCH /api/v1/ui-prefs):
//
// - localStorage stays the synchronous cache, so the first paint reads it
//   exactly as before, with no flicker and no loading state;
// - syncUiPrefs() runs once after sign-in: a value the server already has
//   replaces the local one; a local value the server lacks is uploaded
//   (this is also the migration path for existing installs);
// - setSyncedItem/removeSyncedItem write through to both.
//
// Server values are the raw localStorage strings, so nothing is re-parsed.
import { controlApi } from './openapi';

const SYNCED: Array<string | RegExp> = [
    'scenario.hiddenScenarios',
    'scenario.hiddenDefaultsVersion',
];

export const isSyncedKey = (key: string): boolean =>
    SYNCED.some(k => (typeof k === 'string' ? k === key : k.test(key)));

const patch = (prefs: Record<string, string | null>): Promise<unknown> =>
    controlApi((client, headers) => client.PATCH('/api/v1/ui-prefs', { headers, body: { prefs } }))
        .catch(() => undefined); // Offline or an old server: the local copy still works.

export const setSyncedItem = (key: string, value: string): void => {
    localStorage.setItem(key, value);
    if (isSyncedKey(key)) void patch({ [key]: value });
};

export const removeSyncedItem = (key: string): void => {
    localStorage.removeItem(key);
    if (isSyncedKey(key)) void patch({ [key]: null });
};

const localSyncedKeys = (): string[] => {
    const keys: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (key !== null && isSyncedKey(key)) keys.push(key);
    }
    return keys;
};

// Pull the server's copy into localStorage and upload anything only this
// origin has. Listeners already watching `storage` events (the nav, the
// agent overview) are notified of every key that changed.
export const syncUiPrefs = async (): Promise<void> => {
    const res = await controlApi((client, headers) => client.GET('/api/v1/ui-prefs', { headers }))
        .catch(() => undefined);
    if (!res?.success) return;
    const remote: Record<string, unknown> = res.prefs ?? {};

    const changed: string[] = [];
    for (const [key, value] of Object.entries(remote)) {
        if (!isSyncedKey(key) || typeof value !== 'string') continue;
        if (localStorage.getItem(key) !== value) {
            localStorage.setItem(key, value);
            changed.push(key);
        }
    }

    const upload: Record<string, string> = {};
    for (const key of localSyncedKeys()) {
        if (!(key in remote)) upload[key] = localStorage.getItem(key) as string;
    }
    if (Object.keys(upload).length > 0) await patch(upload);

    for (const key of changed) {
        window.dispatchEvent(new StorageEvent('storage', { key }));
    }
};
