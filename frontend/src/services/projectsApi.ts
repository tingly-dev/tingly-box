// Projects (frontend/packages/projects) talks to its own routes under
// /api/v1/projects with a plain fetch. This is the fetch Tingly Box hands it:
// the same base URL and Authorization header as the app's own control API
// (services/openapi.ts), so the typed calls, the event stream and Library
// downloads are all authenticated. Kept small and page-free: the nav (the
// rail's waiting dot) uses it too, eagerly.
import { getControlApiHeaders } from './openapi';
import { getApiBaseUrl } from '@/utils/protocol';

/** Fired when a project is created, changed or deleted, so nav that shows project state refreshes. */
export const PROJECTS_CHANGED_EVENT = 'tingly:projects-changed';

export const notifyProjectsChanged = () => {
    window.dispatchEvent(new Event(PROJECTS_CHANGED_EVENT));
};

export async function projectsFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const req = new Request(input, init);
    const [base, auth] = await Promise.all([getApiBaseUrl(), getControlApiHeaders()]);
    const headers = new Headers(req.headers);
    for (const [name, value] of Object.entries(auth)) {
        if (!headers.has(name)) headers.set(name, value);
    }

    // Projects builds same-origin URLs. In a browser tab that origin is the
    // gateway; the desktop window is served from elsewhere and reaches the
    // gateway on its own port, so the request moves there.
    const url = new URL(req.url);
    if (url.origin !== window.location.origin || base === window.location.origin) {
        return fetch(new Request(req, { headers }));
    }
    const hasBody = req.method !== 'GET' && req.method !== 'HEAD';
    return fetch(`${base}${url.pathname}${url.search}`, {
        method: req.method,
        headers,
        body: hasBody ? await req.arrayBuffer() : undefined,
        signal: req.signal,
    });
}

/** The fields of GET /api/v1/projects the nav reads. */
interface ProjectSummaryLite {
    threads?: { waiting?: number };
}

/** The fields of GET /api/v1/projects/workdirs the nav reads. */
interface WorkdirLite {
    waiting?: number;
}

async function readJSON<T>(path: string, signal?: AbortSignal): Promise<T | undefined> {
    const res = await projectsFetch(path, { signal });
    return res.ok ? ((await res.json()) as T) : undefined;
}

/**
 * Whether anything in Projects waits on the user: a project thread or a
 * working-folder session. One signal for the rail's dot, whichever kind it is.
 */
export async function anythingWaiting(signal?: AbortSignal): Promise<boolean> {
    const [projects, folders] = await Promise.all([
        readJSON<{ projects?: ProjectSummaryLite[] }>('/api/v1/projects', signal),
        readJSON<{ workdirs?: WorkdirLite[] }>('/api/v1/projects/workdirs', signal),
    ]);
    return (projects?.projects ?? []).some((p) => (p.threads?.waiting ?? 0) > 0)
        || (folders?.workdirs ?? []).some((w) => (w.waiting ?? 0) > 0);
}
