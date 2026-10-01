// Whether this page is running inside the Wails desktop window.
//
// Decided from the page's own origin, which is known before any script runs:
// the Wails asset server serves the window from wails://localhost on macOS
// and Linux and from http://wails.localhost on Windows (wails v3
// internal/assetserver baseURL, also in dev mode, where it proxies to vite).
// window._wails is no signal: the shell injects it only after the page has
// finished loading, by which time this module has long been evaluated.
export function isDesktopShell(location: Pick<Location, 'protocol' | 'hostname'> = window.location): boolean {
    return location.protocol === 'wails:' || location.hostname === 'wails.localhost';
}
