import type { Provider } from '@/types/provider';

// Virtual models route through other providers; they do not supply credentials.
export const isCredentialProvider = (provider: Pick<Provider, 'auth_type'>): boolean =>
    provider.auth_type !== 'vmodel';
