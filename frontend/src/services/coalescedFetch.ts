// A fetch that lets identical GETs share one network request while they are
// in flight.
//
// Several providers, hooks and pages each ask for the same list when they
// mount (providers, /config, the token, the claude status …), so a single
// page load used to send the same GET up to seven times. Callers keep asking
// independently — no shared store to wire up — and the transport folds the
// concurrent duplicates into one request.
//
// Deliberately not a cache: an entry lives only until its request settles,
// and any non-GET (a mutation) drops every entry, both when it starts and
// when it settles, so a read issued after a write never joins a read that
// began before the write landed. Each caller gets its own Response clone, so
// bodies are read (and mutated, e.g. sorted in place) independently.

const inflight = new Map<string, Promise<Response>>();

// Bumped whenever a mutation starts or settles. A cached read taken under an
// older epoch may predate a write and is never served (see readCache.ts).
let epoch = 0;
export const mutationEpoch = (): number => epoch;

const keyOf = (url: string, headers: Headers): string =>
    `${url}\n${headers.get('Authorization') ?? ''}`;

// Rejects when the caller's own signal aborts, without cancelling the shared
// request the other callers are still waiting on.
const withCallerSignal = (shared: Promise<Response>, signal: AbortSignal | null | undefined): Promise<Response> => {
    if (!signal) return shared;
    if (signal.aborted) return Promise.reject(signal.reason);
    return new Promise<Response>((resolve, reject) => {
        const onAbort = () => reject(signal.reason);
        signal.addEventListener('abort', onAbort, { once: true });
        shared.then(resolve, reject).finally(() => signal.removeEventListener('abort', onAbort));
    });
};

export const coalescedFetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const method = (init?.method ?? (input instanceof Request ? input.method : 'GET')).toUpperCase();
    if (method !== 'GET') {
        inflight.clear();
        epoch += 1;
        return fetch(input, init).finally(() => {
            inflight.clear();
            epoch += 1;
        });
    }

    const request = new Request(input, init);

    const key = keyOf(request.url, request.headers);
    let shared = inflight.get(key);
    if (!shared) {
        // The shared request must outlive any one caller, so it does not carry
        // the first caller's signal.
        const started: Promise<Response> = fetch(request.url, {
            method: 'GET',
            headers: request.headers,
            credentials: request.credentials,
            cache: request.cache,
        });
        shared = started;
        inflight.set(key, started);
        const forget = () => {
            if (inflight.get(key) === started) inflight.delete(key);
        };
        started.then(forget, forget);
    }
    return withCallerSignal(shared.then((res) => res.clone()), request.signal);
};
