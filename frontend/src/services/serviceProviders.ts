import api from "@/services/api.ts";

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
}

export interface ServiceProviderOption {
    title: string;
    value: string;
    api_style: string;
    baseUrl: string;
}

const serviceProviders = await async function () {
    const {providersApi} = await api.instances()
    let res = await providersApi.apiV2ProviderTemplatesGet()
    return res.data.data
}()

// Get dropdown options for service provider selection
export function getServiceProviderOptions(): ServiceProviderOption[] {
    const options: ServiceProviderOption[] = [];

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
}

// Get all unique providers (not split by API style)
export function getAllUniqueProviders(): UniqueProvider[] {
    const providers: UniqueProvider[] = [];

    Object.entries(serviceProviders).forEach(([_key, provider]: [string, any]) => {
        const sp = provider as ServiceProvider;

        // Skip OAuth-only providers
        if (sp.auth_type === 'api_key' || sp.oauth_provider) {
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
        });
    });

    // Sort by display name
    providers.sort((a, b) => (a.alias || a.name).localeCompare(b.alias || b.name));

    return providers;
}

// Export the raw data for direct access
export {serviceProviders};
