import api from "@/services/api.ts";
import React from "react";

export interface ServiceProvider {
    id: string;
    name: string;
    alias?: string; // Display name with locale information
    status: string;
    valid: boolean;
    website: string;
    description: string;
    type: string;
    api_doc: string;
    model_doc: string;
    pricing_doc: string;
    base_url_openai?: string;
    base_url_anthropic?: string;
    auth_type?: string;
    oauth_provider?: string;
    icon?: string; // Icon identifier for Lobe Icons (e.g., "openai", "anthropic")
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

// Load provider templates from API
async function loadProviderTemplates(): Promise<Record<string, ServiceProvider>> {
    if (cachedProviders) {
        return cachedProviders;
    }

    // Return existing promise if loading is in progress
    if (loadPromise) {
        return loadPromise!;
    }

    loadPromise = (async (): Promise<Record<string, ServiceProvider>> => {
        try {
            const res = await api.getProviderTemplates();
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
    return loadProviderTemplates();
}

// Synchronous getter for cached providers (returns empty object if not loaded)
export function getServiceProvidersSync(): Record<string, ServiceProvider> {
    return cachedProviders || {};
}

// Initialize provider templates on module load
// This ensures the provider list is available when components mount
getServiceProviders().then(data => {
    const providerCount = Object.keys(data || {}).length;
    console.log(`[serviceProviders] Loaded ${providerCount} provider templates`);
}).catch(err => console.error('Failed to initialize provider templates:', err));

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
    icon?: string; // Icon identifier for Lobe Icons
}

// Get all unique providers (not split by API style)
export function getAllUniqueProviders(): UniqueProvider[] {
    const providers: UniqueProvider[] = [];
    const serviceProviders = getServiceProvidersSync();

    Object.entries(serviceProviders).forEach(([_key, provider]: [string, any]) => {
        const sp = provider as ServiceProvider;

        // Skip OAuth providers - they should be added via the OAuth dialog, not API key dialog
        if (sp.oauth_provider) {
            return;
        }

        providers.push({
            id: sp.id,
            name: sp.name,
            alias: sp.alias,
            supportsOpenAI: !!sp.base_url_openai,
            supportsAnthropic: !!sp.base_url_anthropic,
            baseUrlOpenAI: sp.base_url_openai,
            baseUrlAnthropic: sp.base_url_anthropic,
            icon: sp.icon,
        });
    });

    // Sort by display name
    providers.sort((a, b) => (a.alias || a.name).localeCompare(b.alias || b.name));

    return providers;
}

// React hook for provider templates
// This ensures components re-render when providers are loaded
export function useProviderTemplates(): UniqueProvider[] {
    const [, forceUpdate] = React.useReducer(x => x + 1, 0);

    React.useEffect(() => {
        // Subscribe to provider updates
        const unsubscribe = subscribeToProviders(() => {
            forceUpdate();
        });
        return unsubscribe;
    }, []);

    return getAllUniqueProviders();
}
