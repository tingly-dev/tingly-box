import { mutationEpoch } from './coalescedFetch';

// A few control-plane reads are asked for again and again while one page
// loads, a moment apart, by components that each own their fetch (the agent
// pages and their plugin row both read the same scenario config; the
// dashboard and its tables both read the provider list). In-flight sharing
// (coalescedFetch) only catches asks that overlap, so these reads are also
// reused for a few seconds.
//
// Safe against our own writes: an entry is dropped as soon as any mutation
// goes through the API client (the epoch moves), so a read after a write
// always reaches the server. Only a change made elsewhere — another tab, the
// CLI — can show up up to MAX_AGE_MS late. Every caller gets its own copy, so
// code that sorts or edits the result in place cannot corrupt the others.

const MAX_AGE_MS = 5_000;

interface Entry {
    epoch: number;
    at: number;
    promise: Promise<any>;
}

const entries = new Map<string, Entry>();

/** `fresh` skips the stored copy (a user pressing refresh wants the server's answer). */
export function readCached<T>(key: string, load: () => Promise<T>, { fresh = false } = {}): Promise<T> {
    const hit = entries.get(key);
    let entry: Entry;
    if (!fresh && hit && hit.epoch === mutationEpoch() && Date.now() - hit.at < MAX_AGE_MS) {
        entry = hit;
    } else {
        entry = { epoch: mutationEpoch(), at: Date.now(), promise: load() };
        entries.set(key, entry);
        // A failed read (controlApi reports failure as { success: false }) must not be served again.
        const started = entry;
        const forget = () => { if (entries.get(key) === started) entries.delete(key); };
        started.promise.then((result) => { if (result?.success === false) forget(); }, forget);
    }
    return entry.promise.then((result) => (result && typeof result === 'object' ? structuredClone(result) : result));
}
