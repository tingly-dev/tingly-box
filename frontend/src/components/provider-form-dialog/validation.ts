export const isHttpUrl = (value: string): boolean => {
    try {
        const url = new URL(value.trim());
        return ['http:', 'https:'].includes(url.protocol) && !!url.hostname;
    } catch { return false; }
};
