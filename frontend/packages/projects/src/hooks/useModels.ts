import { useEffect, useState } from 'react';
import { api, unwrap, type Model } from '../client';

/**
 * Where a model list comes from: `connection` is the standalone app's saved
 * connection (Chats); `projects` is what the host offers project settings,
 * which in Tingly Box is its gateway rather than this app's connection.
 */
export type ModelSource = 'connection' | 'projects';

type Listing = { models: Model[]; error?: string };

const cache = new Map<ModelSource, Listing>();
const inflight = new Map<ModelSource, Promise<Listing>>();

function load(source: ModelSource): Promise<Listing> {
    const req = source === 'projects' ? api.GET('/api/v1/projects/models') : api.GET('/api/v1/models');
    return unwrap(req)
        .then((r) => ({ models: r.models, error: r.error }))
        .catch((e: Error) => ({ models: [], error: e.message }));
}

/** Models offered by source, fetched once per page load. */
export function useModels(source: ModelSource = 'connection') {
    const [state, setState] = useState(() => cache.get(source) ?? null);
    useEffect(() => {
        const hit = cache.get(source);
        if (hit) {
            setState(hit);
            return;
        }
        let p = inflight.get(source);
        if (!p) {
            p = load(source);
            inflight.set(source, p);
        }
        let alive = true;
        p.then((r) => {
            cache.set(source, r);
            if (alive) setState(r);
        });
        return () => {
            alive = false;
        };
    }, [source]);
    return { models: state?.models ?? [], error: state?.error, loading: !state };
}

/** Drop the cached lists, e.g. after the connection settings change. */
export function invalidateModels() {
    cache.clear();
    inflight.clear();
}
