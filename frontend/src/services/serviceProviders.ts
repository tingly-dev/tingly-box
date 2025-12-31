import api from "@/services/api.ts";

export interface ServiceProvider {
    id: string;
    name: string;
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

        // If provider supports both APIs, create separate options for each
        if (hasOpenAi) {
            options.push({
                title: `${provider.name}`,
                value: `${provider.id}:openai`,
                api_style: 'openai',
                baseUrl: (provider as ServiceProvider).base_url_openai!
            });
        }
        if (hasAnthropic) {
            options.push({
                title: `${provider.name}`,
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

// Export the raw data for direct access
export {serviceProviders};
