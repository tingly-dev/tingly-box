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

/**
 * Whether a request body can be handed to a second fetch as-is. FormData,
 * Blob/File, strings, URLSearchParams and buffers are values the browser
 * serializes per send — File/Blob contents are streamed from disk, not copied
 * into the heap. A ReadableStream (or Node's async-iterable extension) is
 * consumed by one send, so it cannot be replayed.
 */
function isReplayableBody(body: BodyInit | null | undefined): body is BodyInit {
    if (body == null) return false;
    if (typeof ReadableStream !== 'undefined' && body instanceof ReadableStream) return false;
    if (typeof (body as { [Symbol.asyncIterator]?: unknown })[Symbol.asyncIterator] === 'function') return false;
    return true;
}

/** Whether the caller set Content-Type itself, rather than `new Request` deriving it from the body. */
function explicitContentType(headers: HeadersInit | undefined): boolean {
    return headers != null && new Headers(headers).has('Content-Type');
}

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
    const target = `${base}${url.pathname}${url.search}`;
    if (req.method === 'GET' || req.method === 'HEAD') {
        return fetch(target, { method: req.method, headers, signal: req.signal });
    }

    // Replay the body rather than reading it into memory. Uploads reach here
    // as a FormData in `init.body`; handing the same FormData to fetch streams
    // each File from disk, while `await req.arrayBuffer()` would hold the
    // whole body in the heap (twice for a large upload).
    const body = init?.body;
    if (isReplayableBody(body)) {
        // `req.headers` carries the multipart Content-Type `new Request`
        // derived from one serialization, boundary included. A second send
        // re-serializes the FormData with a new boundary, so a copied header
        // would not match the body. Let fetch generate it, unless the caller
        // set Content-Type itself.
        if (typeof FormData !== 'undefined' && body instanceof FormData && !explicitContentType(init?.headers)) {
            headers.delete('Content-Type');
        }
        return fetch(target, { method: req.method, headers, body, signal: req.signal });
    }

    // A Request input (the typed openapi-fetch client) has no raw body to
    // replay; its bodies are JSON, so buffering them keeps the old behavior
    // without a large-file cost.
    return fetch(target, { method: req.method, headers, body: await req.arrayBuffer(), signal: req.signal });
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
