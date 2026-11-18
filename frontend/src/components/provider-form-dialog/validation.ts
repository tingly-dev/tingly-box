export const isHttpUrl = (value: string): boolean => {
    try {
        const url = new URL(value.trim());
        return ['http:', 'https:'].includes(url.protocol) && !!url.hostname;
    } catch { return false; }
};

/** Empty means "no proxy"; otherwise the schemes the gateway can dial. */
export const isProxyUrl = (value: string): boolean => {
    const trimmed = value.trim();
    if (!trimmed) return true;
    try {
        const url = new URL(trimmed);
        return ['http:', 'https:', 'socks5:'].includes(url.protocol) && !!url.hostname;
    } catch { return false; }
};
