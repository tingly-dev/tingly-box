// The HostBridge for wherever this page is running — see ./types.ts.
// One build serves both hosts; the choice is made here, once, at startup.
import { browserHost } from './browser';
import { createDesktopHost } from './desktop';
import { isDesktopShell } from './detect';
import { routeExternalLinks } from './externalLinks';
import type { HostBridge } from './types';

export const host: HostBridge = isDesktopShell() ? createDesktopHost() : browserHost;

// The desktop window cannot follow external links itself; send them all to
// the OS browser (see ./externalLinks.ts). A browser tab needs nothing.
if (host.kind === 'desktop') routeExternalLinks(window, host.openExternal);
export type { HostBridge } from './types';
