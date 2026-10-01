// HostBridge for a plain browser tab (`tb open`, team deployments, dev, mock,
// tests). The desktop counterpart is ./desktop.ts; see ./types.ts.
import type { HostBridge } from './types';
import { saveFileViaAnchor } from './saveFileViaAnchor';

export const browserHost: HostBridge = {
    kind: 'browser',
    // The page is served by the gateway itself, so its origin is the API.
    gatewayPort: async () => null,
    shellAuthToken: async () => null,
    openMainWindow: async () => {},
    openExternal: (url) => {
        window.open(url, '_blank', 'noopener,noreferrer');
    },
    saveFile: saveFileViaAnchor,
};
