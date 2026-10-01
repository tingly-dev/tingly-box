// HostBridge is the one seam between the UI and whatever is hosting it:
// a plain browser tab (`tb open`, team deployments) or the Wails desktop
// window. Pages ask the bridge for a capability; they never branch on the
// host themselves. One build serves both; ./index.ts picks the implementation
// at startup from the page's origin (./detect.ts). See
// .design/ui-redesign.md §4.2.
export interface HostBridge {
    kind: 'browser' | 'desktop';
    /** Port of the in-process gateway, or null when the page's own origin is the gateway. */
    gatewayPort(): Promise<number | null>;
    /** Auth token the desktop shell hands the UI when none is stored; null in a browser. */
    shellAuthToken(): Promise<string | null>;
    /** Show the desktop main window at path (tray hub panel); no-op in a browser. */
    openMainWindow(path: string): Promise<void>;
    /** Open a URL outside the app, in the user's browser. */
    openExternal(url: string): void;
    /** Save a blob to the user's disk under the given file name. */
    saveFile(blob: Blob, fileName: string): void;
}
