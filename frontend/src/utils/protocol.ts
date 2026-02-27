// Centralized protocol and URL handling for GUI (Wails) and Web modes
import TinglyService from "@/bindings";

/**
 * Runtime mode enumeration
 */
export enum RuntimeMode {
  GUI = 'gui',       // Wails desktop app
  WEB = 'web',       // Production web deployment
  DEV = 'dev',       // Development with Vite dev server
}

/**
 * Get the current runtime mode
 */
export function getRuntimeMode(): RuntimeMode {
  const pkgMode = import.meta.env.VITE_PKG_MODE;
  if (pkgMode === 'gui') return RuntimeMode.GUI;
  if (pkgMode === 'ui') return RuntimeMode.DEV;
  return RuntimeMode.WEB;
}

/**
 * Get the API protocol for backend communication
 * - GUI mode: Always 'http:' (localhost communication)
 * - Web mode: Use window.location.protocol (http: or https:)
 */
export function getApiProtocol(): string {
  const mode = getRuntimeMode();
  if (mode === RuntimeMode.GUI) {
    return 'http:';
  }
  return window.location.protocol;
}

/**
 * Get the base URL for API calls
 * - GUI mode: http://localhost:{port} from Wails service
 * - Web mode: {protocol}//{host}
 */
export async function getApiBaseUrl(): Promise<string> {
  const mode = getRuntimeMode();
  const protocol = getApiProtocol();

  if (mode === RuntimeMode.GUI) {
    const port = await TinglyService.GetPort();
    return `${protocol}//localhost:${port}`;
  }

  const host = window.location.host.replace(/\/$/, '');
  return `${protocol}//${host}`;
}

/**
 * Get the origin/protocol for display purposes
 * - GUI mode: Return 'wails:' for accurate display
 * - Web mode: Return window.location.origin
 */
export function getDisplayOrigin(): string {
  const mode = getRuntimeMode();
  if (mode === RuntimeMode.GUI) {
    return 'wails://';
  }
  return window.location.origin;
}

/**
 * Check if running in HTTPS mode
 */
export function isHttps(): boolean {
  return getRuntimeMode() !== RuntimeMode.GUI && window.location.protocol === 'https:';
}

/**
 * Check if running in GUI mode
 */
export function isGuiMode(): boolean {
  return getRuntimeMode() === RuntimeMode.GUI;
}

/**
 * Get the OAuth redirect URI for callback
 * - GUI mode: http://localhost:{port}/oauth/callback (local callback)
 * - Web mode: {origin}/oauth/callback
 */
export async function getOAuthRedirectPath(): Promise<string> {
  const mode = getRuntimeMode();
  if (mode === RuntimeMode.GUI) {
    const port = await TinglyService.GetPort();
    return `http://localhost:${port}/oauth/callback`;
  }
  return `${window.location.origin}/oauth/callback`;
}
