import api from "@/services/api.ts";
import React from "react";
import {isCloudAuthType} from "@/components/cloud/cloudCredentialSchema";

export interface ServiceProvider {
    id: string;
    name: string;
    alias?: string; // Display name with locale information
    status: string;
    valid: boolean;
    website: string;
    description: string;
    canonical_domain?: string; // API host (Schema V2)
    vendor_family?: string;    // Vendor aggregation key (Schema V2)
    region?: string;           // "cn" | "intl" | "global" (Schema V2)
    plan?: string;             // "standard" | "coding" | "oauth" (Schema V2)
    api_doc: string;
    model_doc: string;
    pricing_doc: string;
    base_url_openai?: string;
    base_url_anthropic?: string;
    api_style?: string; // Explicit protocol ("openai" | "anthropic" | "google"); cloud templates set this
    auth_type?: string;
    oauth_provider?: string;
    icon?: string; // Icon identifier for Lobe Icons (e.g., "openai", "anthropic")
    type?: string;  // Provider type: "official" | "reseller" | "self-hosted" | "cloud" | ...
}

export interface ServiceProviderOption {
    title: string;
    value: string;
    api_style: string;
    baseUrl: string;
}

// Cache for provider templates
let cachedProviders: Record<string, ServiceProvider> | null = null;
let loadPromise: Promise<Record<string, ServiceProvider>> | null = null;

// Listener mechanism for provider template loading
type ProviderListener = () => void;
const listeners: Set<ProviderListener> = new Set();

export function subscribeToProviders(listener: ProviderListener): () => void {
    listeners.add(listener);
    // If already loaded, notify immediately
    if (cachedProviders) {
        listener();
    }
    // Return unsubscribe function
    return () => listeners.delete(listener);
}

function notifyListeners() {
    listeners.forEach(listener => listener());
}

// Load provider catalog entries from API
async function loadProviderCatalogs(): Promise<Record<string, ServiceProvider>> {
    if (cachedProviders) {
        return cachedProviders;
    }

    // Return existing promise if loading is in progress
    if (loadPromise) {
        return loadPromise!;
    }

    loadPromise = (async (): Promise<Record<string, ServiceProvider>> => {
        try {
            const res = await api.getProviderCatalogs();
            if (res && res.success && res.data) {
                cachedProviders = res.data;
                notifyListeners(); // Notify all subscribers
                return cachedProviders!;
            }
        } catch (error) {
            console.error('Failed to load provider templates:', error);
        } finally {
            loadPromise = null; // Clear promise after completion
        }

        return {} as Record<string, ServiceProvider>;
    })();

    return loadPromise!;
}

// Export a function to get service providers (lazy loading)
export async function getServiceProviders(): Promise<Record<string, ServiceProvider>> {
    return loadProviderCatalogs();
}

// Synchronous getter for cached providers (returns empty object if not loaded)
export function getServiceProvidersSync(): Record<string, ServiceProvider> {
    return cachedProviders || {};
}

// Initialize provider templates on module load
// This ensures the provider list is available when components mount
getServiceProviders().catch(err => console.error('Failed to initialize provider templates:', err));

// Get dropdown options for service provider selection
export function getServiceProviderOptions(): ServiceProviderOption[] {
    const options: ServiceProviderOption[] = [];
    const serviceProviders = getServiceProvidersSync();

    Object.entries(serviceProviders).forEach(([key, provider]: [string, any]) => {
        const hasOpenAi = !!(provider as ServiceProvider).base_url_openai;
        const hasAnthropic = !!(provider as ServiceProvider).base_url_anthropic;

        // Use alias if available, otherwise fallback to name
        const displayName = (provider as ServiceProvider).alias || (provider as ServiceProvider).name;

        // If provider supports both APIs, create separate options for each
        if (hasOpenAi) {
            options.push({
                title: displayName,
                value: `${provider.id}:openai`,
                api_style: 'openai',
                baseUrl: (provider as ServiceProvider).base_url_openai!
            });
        }
        if (hasAnthropic) {
            options.push({
                title: displayName,
                value: `${provider.id}:anthropic`,
                api_style: 'anthropic',
                baseUrl: (provider as ServiceProvider).base_url_anthropic!
            });
        }
    });

    // Sort by name
    options.sort((a, b) => a.title.localeCompare(b.title));

    return options;
}

// Get provider by ID
export function getServiceProvider(id: string): ServiceProvider | null {
    const serviceProviders = getServiceProvidersSync();
    const provider = (serviceProviders as any)[id];
    return provider || null;
}

// Get provider options filtered by API style
export function getProvidersByStyle(style: 'openai' | 'anthropic'): ServiceProviderOption[] {
    return getServiceProviderOptions().filter(option => option.api_style === style);
}

// Unique provider representation (not duplicated by style)
export interface UniqueProvider {
    id: string;
    name: string;
    alias?: string;
    supportsOpenAI: boolean;
    supportsAnthropic: boolean;
    baseUrlOpenAI?: string;
    baseUrlAnthropic?: string;
    website?: string;
    apiDoc?: string;
    icon?: string; // Icon identifier for Lobe Icons
    region?: 'cn' | 'global' | 'self-hosted'; // Derived region grouping for UI
    type?: string; // Provider type: "official" | "reseller" | "self-hosted" | etc.
    authType?: string; // "aws_sigv4" | "gcp_sa" | "azure_key" | ...; carried for cloud grouping
    apiStyle?: string; // Explicit protocol for cloud providers ("anthropic" | "openai" | "google")
    description?: string; // Short description, used as card subtitle for cloud providers
}

// Heuristic classification of a provider into "cn" (China) or "global".
// Prefers explicit region from Schema V2 when present; otherwise falls back
// to id/name patterns (e.g. "(CN)" suffix, "-speciale" coding-only CN endpoint).
function classifyRegion(sp: ServiceProvider): 'cn' | 'global' | 'self-hosted' {
    const explicit = (sp.region || '').toLowerCase();
    if (explicit === 'self-hosted') return 'self-hosted';
    if (explicit === 'cn') return 'cn';
    if (explicit === 'intl' || explicit === 'global') return 'global';

    const id = (sp.id || '').toLowerCase();
    const name = (sp.name || '').toLowerCase();
    if (name.includes('(cn)') || name.includes('（cn）')) return 'cn';
    if (id.endsWith('-speciale')) return 'cn';
    if (id.endsWith('-cn')) return 'cn';
    return 'global';
}

// A template belongs in the Cloud picker section when it is typed "cloud" or
// carries a known multi-field auth type. Checking both means a future cloud
// template (e.g. azure_entra) marked type:"cloud" never silently lands in the
// API-key list, even before the frontend learns its credential schema.
function isCloudTemplate(sp: ServiceProvider): boolean {
    return sp.type === 'cloud' || isCloudAuthType(sp.auth_type);
}

// Shared ServiceProvider → UniqueProvider mapping used by both picker lists.
function toUniqueProvider(sp: ServiceProvider): UniqueProvider {
    return {
        id: sp.id,
        name: sp.name,
        alias: sp.alias,
        supportsOpenAI: !!sp.base_url_openai,
        supportsAnthropic: !!sp.base_url_anthropic,
        baseUrlOpenAI: sp.base_url_openai,
        baseUrlAnthropic: sp.base_url_anthropic,
        website: sp.website,
        apiDoc: sp.api_doc,
        icon: sp.icon,
        region: classifyRegion(sp),
        type: sp.type,
        authType: sp.auth_type,
        apiStyle: sp.api_style,
        description: sp.description,
    };
}

function sortByDisplayName(providers: UniqueProvider[]): UniqueProvider[] {
    providers.sort((a, b) => (a.alias || a.name).localeCompare(b.alias || b.name));
    return providers;
}

// Get all unique providers (not split by API style)
export function getAllUniqueProviders(): UniqueProvider[] {
    const seen = new Map<string, UniqueProvider>();

    Object.values(getServiceProvidersSync()).forEach((provider) => {
        const sp = provider as ServiceProvider;

        // OAuth and cloud-credential templates have their own picker sections
        // and dialogs; neither belongs in the protocol-slot API-key list.
        if (sp.oauth_provider || isCloudTemplate(sp)) {
            return;
        }

        // Use provider.id as the dedup key. When the source dictionary has
        // two entries with the same id (e.g. a coding-plan variant keyed
        // differently but sharing the logical id), the first one seen wins.
        if (!seen.has(sp.id)) {
            seen.set(sp.id, toUniqueProvider(sp));
        }
    });

    return sortByDisplayName(Array.from(seen.values()));
}

// Cloud-credential providers (Bedrock / Vertex / Azure) for the "Cloud" picker
// section; carries authType, apiStyle and description so the cloud dialog can
// build the right credential form.
export function getCloudProviders(): UniqueProvider[] {
    const out = Object.values(getServiceProvidersSync())
        .filter((sp) => isCloudTemplate(sp as ServiceProvider))
        .map((sp) => toUniqueProvider(sp as ServiceProvider));
    return sortByDisplayName(out);
}

// Unified search function for provider templates.
// Searches across all user-searchable fields so providers are discoverable
// by their common names, even when the display alias differs.
export function searchProviders(providers: UniqueProvider[], query: string): UniqueProvider[] {
    const needle = query.trim().toLowerCase();
    if (!needle) {
        return providers;
    }

    return providers.filter(provider => {
        const displayName = (provider.alias || provider.name).toLowerCase();
        const fields = [
            displayName,
            provider.id || '',
            provider.name || '',
            provider.baseUrlOpenAI || '',
            provider.baseUrlAnthropic || '',
            provider.website || '',
            // Additional discovery fields — users often search by these
            provider.icon || '',
            provider.type || '',
            provider.description || '',
            provider.authType || '',
        ];
        return fields.some(f => f.toLowerCase().includes(needle));
    });
}

// Re-render the calling component when the provider templates load/refresh,
// then return the selector's current value. Shared by all template hooks so
// the subscription lifecycle exists once.
function useProviderSelector<T>(select: () => T): T {
    const [, forceUpdate] = React.useReducer(x => x + 1, 0);

    React.useEffect(() => {
        return subscribeToProviders(forceUpdate);
    }, []);

    return select();
}

// React hook for provider catalog entries (API-key picker list).
export function useProviderCatalogs(): UniqueProvider[] {
    return useProviderSelector(getAllUniqueProviders);
}

// Reactive accessor for cloud-credential provider catalog entries (Cloud picker section).
export function useCloudProviders(): UniqueProvider[] {
    return useProviderSelector(getCloudProviders);
}
