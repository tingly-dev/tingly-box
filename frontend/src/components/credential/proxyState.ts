export type ProxyState = 'none' | 'on' | 'off';

interface ProxyFields {
    proxy_url?: string | null;
    proxy_disabled?: boolean | null;
}

/** none: no proxy URL stored; off: stored but switched off; on: in use. */
export function proxyState(provider: ProxyFields): ProxyState {
    if (!provider.proxy_url) return 'none';
    return provider.proxy_disabled ? 'off' : 'on';
}

/** The proxy actually used on the wire; empty when none or switched off. */
export function effectiveProxyUrl(provider: ProxyFields): string {
    return proxyState(provider) === 'on' ? provider.proxy_url ?? '' : '';
}

/** Hide userinfo so a list tooltip never shows proxy credentials. */
export function redactProxyUrl(raw: string): string {
    try {
        const u = new URL(raw);
        if (!u.username && !u.password) return raw;
        return `${u.protocol}//***@${u.host}`;
    } catch {
        return raw;
    }
}
