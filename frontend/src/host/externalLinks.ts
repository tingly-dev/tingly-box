// Routes every external link in the desktop window to the OS browser.
//
// A WebView has nowhere to put a new tab: <a target="_blank"> is swallowed
// (GitHub star, repo, docs and release links did nothing), and an external
// link without a target navigates the app window itself away from the app.
// Rather than make every call site remember host.openExternal, the desktop
// window intercepts the clicks once, here — links added later are covered
// without anyone having to know.
//
// Installed only in the desktop shell (./index.ts). In-app links (same
// origin) and clicks a handler already prevented are left alone; the
// listener runs in the bubble phase on document, after React's root
// listener, so a component's own preventDefault wins.

const EXTERNAL_PROTOCOLS = new Set(['http:', 'https:', 'mailto:']);

/** The absolute URL to hand to the OS browser, or null for an in-app link. */
export function externalHref(href: string, location: Pick<Location, 'href' | 'origin'>): string | null {
    let url: URL;
    try {
        url = new URL(href, location.href);
    } catch {
        return null;
    }
    if (!EXTERNAL_PROTOCOLS.has(url.protocol)) return null;
    if (url.protocol !== 'mailto:' && url.origin === location.origin) return null;
    return url.href;
}

export function routeExternalLinks(win: Window, openExternal: (url: string) => void): () => void {
    const doc = win.document;

    const onClick = (event: MouseEvent) => {
        // Primary and middle button (middle-click means "new tab" in a browser).
        if (event.defaultPrevented || (event.button !== 0 && event.button !== 1)) return;
        const anchor = (event.target as Element | null)?.closest?.('a[href]') as HTMLAnchorElement | null;
        if (!anchor || anchor.hasAttribute('download')) return;
        const url = externalHref(anchor.getAttribute('href') ?? '', win.location);
        if (!url) return;
        event.preventDefault();
        openExternal(url);
    };
    doc.addEventListener('click', onClick);
    doc.addEventListener('auxclick', onClick);

    // window.open from code (and libraries) gets the same treatment.
    const originalOpen = win.open;
    win.open = ((url?: string | URL, ...rest: unknown[]) => {
        const external = url === undefined ? null : externalHref(String(url), win.location);
        if (external) {
            openExternal(external);
            return null;
        }
        return (originalOpen as (...a: unknown[]) => Window | null).call(win, url, ...rest);
    }) as typeof win.open;

    return () => {
        doc.removeEventListener('click', onClick);
        doc.removeEventListener('auxclick', onClick);
        win.open = originalOpen;
    };
}
