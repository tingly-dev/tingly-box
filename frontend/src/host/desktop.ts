// HostBridge for the Wails desktop window. Only this module talks to the
// Wails runtime; see ./types.ts.
//
// The runtime is loaded with a dynamic import, never a static one: importing
// @wailsio/runtime registers global context-menu and drag handlers that, in a
// plain browser tab, would suppress the right-click menu and intercept mouse
// events. ./index.ts only calls createDesktopHost() inside the desktop shell,
// so a browser never even downloads the runtime chunk.
import type { HostBridge } from './types';
import { saveFileViaAnchor } from './saveFileViaAnchor';

// Bound Go methods are called by their fully-qualified name instead of
// through generated bindings, so building the frontend needs no wails3 CLI.
// The names are a contract with gui/wails3/services/tingly_service.go:
// desktop.contract.test.ts fails if one of these methods is renamed there.
export const TINGLY_SERVICE = 'github.com/tingly-dev/tingly-box/gui/wails3/services.TinglyService';
export const BOUND_METHODS = {
    getPort: `${TINGLY_SERVICE}.GetPort`,
    getUserAuthToken: `${TINGLY_SERVICE}.GetUserAuthToken`,
    openMainWindow: `${TINGLY_SERVICE}.OpenMainWindow`,
} as const;

// GUI-only routes, registered in TinglyService.ServiceStartup: save through
// a native dialog, and open a URL in the OS browser.
export const SAVE_FILE_ROUTE = '/api/v1/gui/save';
export const OPEN_URL_ROUTE = '/api/v1/gui/open-url';

type WailsRuntime = typeof import('@wailsio/runtime');

export function createDesktopHost(
    loadRuntime: () => Promise<WailsRuntime> = () => import('@wailsio/runtime'),
): HostBridge {
    // Start loading right away rather than on first use: the runtime also
    // wires the window's drag regions and default context menu, and tells the
    // shell it is ready — all of which should happen at startup, as they did
    // when the runtime was imported statically.
    //
    // The shell never pushes navigation over the Wails event bridge: the tray
    // loads the main window at a URL instead (gui/wails3/window.go), because
    // events proved unreliable in these WebViews.
    const runtime = loadRuntime();

    return {
        kind: 'desktop',
        gatewayPort: async () => (await runtime).Call.ByName(BOUND_METHODS.getPort),
        shellAuthToken: async () =>
            ((await (await runtime).Call.ByName(BOUND_METHODS.getUserAuthToken)) as string) || null,
        openMainWindow: async (path) => {
            await (await runtime).Call.ByName(BOUND_METHODS.openMainWindow, path);
        },
        // A WebView has no tab strip to open `_blank` into; hand the URL to
        // the OS browser instead. Every external link in the window lands
        // here (./externalLinks.ts). HTTP first, like saveFile; the runtime's
        // own Browser.OpenURL (Wails IPC) only as the fallback.
        openExternal: (url) => {
            void shellPost(OPEN_URL_ROUTE, {
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ url }),
            }).catch(async (error) => {
                console.error('Opening the link through the shell failed, trying the runtime:', error);
                await (await runtime).Browser.OpenURL(url);
            });
        },
        saveFile: (blob, fileName) => {
            void saveViaShell(blob, fileName).catch((error) => {
                console.error('Native save failed, falling back to a download link:', error);
                saveFileViaAnchor(blob, fileName);
            });
        },
    };

    // The WebView has no download handling (wails v3 wires no download
    // delegate, so an <a download> click does nothing in WKWebView). The
    // shell shows a native Save dialog and writes the file instead:
    // gui/wails3/services/save_file.go. Over the gateway's HTTP port, the
    // path every API call takes, not the Wails IPC bridge (see window.go on
    // why IPC is avoided). A cancelled dialog resolves normally — only a
    // failed request falls back to the anchor.
    async function saveViaShell(blob: Blob, fileName: string): Promise<void> {
        await shellPost(`${SAVE_FILE_ROUTE}?name=${encodeURIComponent(fileName)}`, { body: blob });
    }

    // POST to one of the GUI-only routes on the gateway's port, with the
    // shell's user token.
    async function shellPost(route: string, init: { headers?: Record<string, string>; body: BodyInit }): Promise<void> {
        const { Call } = await runtime;
        const [port, token] = await Promise.all([
            Call.ByName(BOUND_METHODS.getPort) as Promise<number>,
            Call.ByName(BOUND_METHODS.getUserAuthToken) as Promise<string>,
        ]);
        const response = await fetch(`http://localhost:${port}${route}`, {
            method: 'POST',
            headers: { ...init.headers, Authorization: `Bearer ${token}` },
            body: init.body,
        });
        if (!response.ok) throw new Error(`${route} failed: HTTP ${response.status}`);
    }
}
