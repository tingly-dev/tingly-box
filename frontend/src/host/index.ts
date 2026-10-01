// The HostBridge for wherever this page is running — see ./types.ts.
// One build serves both hosts; the choice is made here, once, at startup.
import { browserHost } from './browser';
import { createDesktopHost } from './desktop';
import { isDesktopShell } from './detect';
import type { HostBridge } from './types';

export const host: HostBridge = isDesktopShell() ? createDesktopHost() : browserHost;
export type { HostBridge } from './types';
