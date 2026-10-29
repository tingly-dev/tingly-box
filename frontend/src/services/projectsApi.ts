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

/** The fields of GET /api/v1/projects the nav reads (projects and folders share the list). */
interface ProjectSummaryLite {
    threads?: { waiting?: number; failed?: number };
}

/**
 * Whether anything in Projects waits on the user: a thread of a project or of
 * a working folder, which are one list. One signal for the rail's dot.
 */
export async function anythingWaiting(signal?: AbortSignal): Promise<boolean> {
    const res = await projectsFetch('/api/v1/projects', { signal });
    if (!res.ok) return false;
    const body = (await res.json()) as { projects?: ProjectSummaryLite[] };
    return (body.projects ?? []).some((p) => (p.threads?.waiting ?? 0) + (p.threads?.failed ?? 0) > 0);
}
