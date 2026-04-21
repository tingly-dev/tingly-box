/**
 * Token and URL transformation utilities.
 * Used across configuration cards and modals.
 */

/**
 * Masks a token for display by showing only first and last 12 characters.
 */
export const maskToken = (token: string): string => {
    if (token.length <= 16) return token;
    const start = token.slice(0, 12);
    const end = token.slice(-12);
    return `${start}${'*'.repeat(8)}${end}`;
};

/**
 * Environment mode type for URL transformation.
 */
export type EnvironmentMode = 'local' | 'docker' | 'cli' | 'npx' | 'wsl';

/**
 * Transforms a URL based on the environment mode.
 * - docker: Replaces host with host.docker.internal
 * - cli/npx/wsl: Returns URL as-is (future: may add specific transformations)
 * - local: Returns URL as-is
 */
export const transformUrlByMode = (url: string, mode: EnvironmentMode): string => {
    switch (mode) {
        case 'docker':
            return url.replace(/\/\/([^/:]+)(?::(\d+))?/, '//host.docker.internal:$2');
        case 'cli':
        case 'npx':
        case 'wsl':
            // Future: implement specific transformations
            return url;
        case 'local':
        default:
            return url;
    }
};
