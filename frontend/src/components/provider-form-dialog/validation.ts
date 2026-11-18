export const isHttpUrl = (value: string): boolean => {
    try {
        const url = new URL(value.trim());
        return ['http:', 'https:'].includes(url.protocol) && !!url.hostname;
    } catch { return false; }
};

/** The schemes the gateway can dial through. */
export const isProxyUrl = (value: string): boolean => {
    try {
        const url = new URL(value.trim());
        return ['http:', 'https:', 'socks5:'].includes(url.protocol) && !!url.hostname;
    } catch { return false; }
};
