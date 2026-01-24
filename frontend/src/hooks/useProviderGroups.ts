import { useMemo } from 'react';
import type { Provider } from '../types/provider';

export interface ProviderGroup {
    authType: string;
    providers: Provider[];
}

export function useProviderGroups(providers: Provider[], singleProvider: Provider | null | undefined) {
    // In single provider mode, use only that provider
    const displayProviders = singleProvider ? [singleProvider] : providers;
    const isSingleProviderMode = singleProvider !== null && singleProvider !== undefined;

    // Memoize enabled providers to avoid repeated filtering
    const enabledProviders = useMemo(
        () => displayProviders.filter(provider => provider.enabled),
        [displayProviders]
    );

    // Group and sort providers by auth_type
    const groupedProviders = useMemo(() => {
        const groups: { [key: string]: Provider[] } = {};
        const authTypeOrder = ['oauth', 'api_key', 'bearer_token', 'basic_auth'];

        enabledProviders.forEach(provider => {
            const authType = provider.auth_type || 'api key';
            if (!groups[authType]) {
                groups[authType] = [];
            }
            groups[authType].push(provider);
        });

        // Sort providers within each group by name
        Object.keys(groups).forEach(authType => {
            groups[authType].sort((a, b) => a.name.localeCompare(b.name));
        });

        // Sort groups by predefined order, then by auth_type name for unknown types
        const sortedGroups: ProviderGroup[] = [];
        authTypeOrder.forEach(authType => {
            if (groups[authType]) {
                sortedGroups.push({ authType, providers: groups[authType] });
                delete groups[authType];
            }
        });

        // Add remaining groups
        Object.keys(groups).sort().forEach(authType => {
            sortedGroups.push({ authType, providers: groups[authType] });
        });

        return sortedGroups;
    }, [enabledProviders]);

    // Flatten grouped providers for tab indexing
    const flattenedProviders = useMemo(
        () => groupedProviders.flatMap(group => group.providers),
        [groupedProviders]
    );

    // Provider name to UUID mapping
    const providerNameToUuid = useMemo(() => {
        const map: { [name: string]: string } = {};
        providers.forEach(provider => {
            map[provider.name] = provider.uuid;
        });
        return map;
    }, [providers]);

    return {
        groupedProviders,
        flattenedProviders,
        providerNameToUuid,
        isSingleProviderMode,
        displayProviders,
    };
}
