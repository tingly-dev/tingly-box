import { useCallback } from 'react';
import { getServiceProvidersSync, useProviderCatalogLoaded } from '@/services/serviceProviders';

/** The slice of a provider the icon lookup reads. */
export interface IconSource {
    api_base?: string;
    oauth_detail?: { provider_type?: string; issuer?: string };
}

// OAuth issuer ids (see components/oauth/fallbackProviders.tsx) whose name
// isn't already a ProviderIcon key.
const OAUTH_ISSUER_ICON: Record<string, string> = {
    claude_code: 'claudecode',
    kimi_code: 'kimi',
    qwen_code: 'qwen',
    antigravity: 'google',
    workbuddy: 'tencent',
    workbuddy_cn: 'tencent',
};

/**
 * Whether an API base is a Tingly-Box route (/tingly/<scenario>, possibly
 * behind a proxy prefix). Mirrors ai/quota's GatewayQuotaURL: the route shape,
 * not the host, identifies a Tingly-Box — an edge reaches its central box on
 * any host. Such a provider uses another Tingly-Box as its upstream.
 */
export function isTinglyBoxBase(apiBase?: string): boolean {
    return /\/tingly\/[^/]+/.test(apiBase ?? '');
}

/**
 * Picks the ProviderIcon identifier for a configured provider from the
 * provider itself: a Tingly-Box upstream by its route, an OAuth provider by its
 * issuer, anything else by its API base's host. Quota is deliberately not
 * consulted — which fetcher read a quota says nothing about who the provider is,
 * and the logo must not change with whether a reading has arrived. Undefined
 * when nothing matches, so the caller can fall back to ProviderIcon's neutral
 * placeholder.
 */
export function providerIconId(provider: IconSource | undefined): string | undefined {
    if (isTinglyBoxBase(provider?.api_base)) return 'tinglybox';
    const issuer = provider?.oauth_detail?.provider_type || provider?.oauth_detail?.issuer;
    if (issuer) return OAUTH_ISSUER_ICON[issuer] ?? issuer;
    return iconIdFromCatalog(provider);
}

function hostOf(apiBase?: string): string {
    const base = (apiBase ?? '').trim();
    if (!base) return '';
    try {
        return new URL(base.includes('://') ? base : `http://${base}`).hostname.toLowerCase();
    } catch {
        return '';
    }
}

/**
 * Last resort for plain API-key providers, which carry no quota type: match the
 * API base's host against the catalog's canonical_domain and base URLs
 * (a host or a dot-separated suffix of it). Several templates can share one host — OAuth
 * products such as Claude Code sit on api.anthropic.com beside the API itself —
 * so OAuth templates are skipped and the longest matching domain wins.
 */
function iconIdFromCatalog(provider: IconSource | undefined): string | undefined {
    const host = hostOf(provider?.api_base);
    if (!host) return undefined;
    let best: { domain: string; icon: string } | undefined;
    for (const template of Object.values(getServiceProvidersSync())) {
        if (!template.icon || template.auth_type === 'oauth') continue;
        const domains = [template.canonical_domain, hostOf(template.base_url_openai), hostOf(template.base_url_anthropic)];
        for (const raw of domains) {
            const domain = raw?.toLowerCase();
            if (!domain || (host !== domain && !host.endsWith(`.${domain}`))) continue;
            if (!best || domain.length > best.domain.length) best = { domain, icon: template.icon };
        }
    }
    return best?.icon;
}

/**
 * providerIconId as a hook: re-renders once the provider catalog has loaded, so
 * a list rendered before that resolves picks up its icons afterwards.
 */
export function useProviderIconId() {
    useProviderCatalogLoaded();
    return useCallback(providerIconId, []);
}
