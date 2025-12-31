import { type ServiceProvider } from './serviceProviders.ts';

export interface ParsedProviderValue {
  providerId: string;
  apiStyle: 'openai' | 'anthropic';
}

export function parseProviderValue(value: string): ParsedProviderValue {
  const [providerId, apiStyle] = value.split(':');
  return {
    providerId,
    apiStyle: apiStyle as 'openai' | 'anthropic'
  };
}

export function formatProviderValue(providerId: string, apiStyle: 'openai' | 'anthropic'): string {
  return `${providerId}:${apiStyle}`;
}

// Get provider's API base URL based on style
export function getProviderBaseUrl(provider: ServiceProvider, apiStyle: 'openai' | 'anthropic'): string {
  if (apiStyle === 'openai' && provider.base_url_openai) {
    return provider.base_url_openai;
  }
  if (apiStyle === 'anthropic' && provider.base_url_anthropic) {
    return provider.base_url_anthropic;
  }

  throw new Error(`Unsupported API style: ${apiStyle}`);
}