import { v4 as uuidv4 } from 'uuid';
import { api } from '@/services/api';
import type { SmartRouting, ConfigProvider, Rule, ConfigRecord } from '@/components/RoutingGraphTypes';

// ============================================================================
// Converter Functions
// ============================================================================

/**
 * Converts a service from Rule format to ConfigProvider format
 */
export function serviceToConfigProvider(service: any): ConfigProvider {
    return {
        uuid: service.id || service.uuid || uuidv4(),
        provider: service.provider || '',
        model: service.model || '',
        isManualInput: false,
        weight: service.weight || 0,
        active: service.active !== undefined ? service.active : true,
        time_window: service.time_window || 0,
    };
}

/**
 * Converts smart routing services to ensure UUID presence
 */
export function normalizeSmartRoutingServices(smartRouting: SmartRouting[]): SmartRouting[] {
    return smartRouting.map((routing) => ({
        ...routing,
        services: (routing.services || []).map((service: ConfigProvider) => ({
            ...service,
            uuid: service.id || service.uuid || uuidv4(),
        })),
    }));
}

/**
 * Converts a Rule to ConfigRecord format
 */
export function ruleToConfigRecord(rule: Rule): ConfigRecord {
    const services = rule.services || [];
    const providersList: ConfigProvider[] = services.map(serviceToConfigProvider);
    const smartRouting = normalizeSmartRoutingServices(rule.smart_routing || []);

    return {
        uuid: rule.uuid || uuidv4(),
        scenario: rule.scenario,
        requestModel: rule.request_model || '',
        responseModel: rule.response_model || '',
        active: rule.active !== undefined ? rule.active : true,
        providers: providersList,
        description: rule.description,
        smartEnabled: rule.smart_enabled || false,
        smartRouting: smartRouting,
    };
}

/**
 * Creates a deep copy of a SmartRouting object
 */
export function cloneSmartRouting(smartRouting: SmartRouting): SmartRouting {
    return {
        uuid: smartRouting.uuid,
        description: smartRouting.description,
        ops: smartRouting.ops.map((op) => ({ ...op })),
        services: smartRouting.services.map((service) => ({ ...service })),
    };
}

/**
 * Creates a new empty SmartRouting object
 */
export function createEmptySmartRouting(): SmartRouting {
    return {
        uuid: crypto.randomUUID(),
        description: 'Smart Routing',
        ops: [],
        services: [],
    };
}

/**
 * Validates if a ConfigRecord is ready for auto-save
 */
export function isConfigRecordReadyForSave(configRecord: ConfigRecord): boolean {
    if (!configRecord.requestModel) return false;
    for (const provider of configRecord.providers) {
        if (provider.provider && !provider.model) {
            return false;
        }
    }
    return true;
}

// ============================================================================
// Export Functions
// ============================================================================

interface ExportMetadata {
    type: 'metadata';
    version: string;
    exported_at: string;
}

interface ExportRule {
    type: 'rule';
    uuid: string;
    scenario: string;
    request_model: string;
    response_model?: string;
    description?: string;
    services: any[];
    active?: boolean;
    smart_enabled?: boolean;
    smart_routing: any[];
}

interface ExportProvider {
    type: 'provider';
    uuid: string;
    name: string;
    api_base: string;
    api_style: string;
    auth_type: string;
    token?: string;
    oauth_detail?: any;
    enabled: boolean;
    proxy_url?: string;
    timeout?: number;
    tags?: string[];
    models?: any[];
}

/**
 * Exports a rule with its associated providers to a JSONL file
 */
export async function exportRuleWithProviders(
    rule: Rule,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    try {
        // Collect unique provider UUIDs from services
        const providerUuids = new Set<string>();
        (rule.services || []).forEach((service: any) => {
            if (service.provider) {
                providerUuids.add(service.provider);
            }
        });

        // Fetch all providers
        const providersData: any[] = [];
        for (const uuid of providerUuids) {
            try {
                const result = await api.getProvider(uuid);
                if (result.success && result.data) {
                    providersData.push(result.data);
                }
            } catch (error) {
                console.error(`Failed to fetch provider ${uuid}:`, error);
            }
        }

        // Build JSONL export
        const lines: string[] = [];

        // Line 1: Metadata
        const metadata: ExportMetadata = {
            type: 'metadata',
            version: '1.0',
            exported_at: new Date().toISOString(),
        };
        lines.push(JSON.stringify(metadata));

        // Line 2: Rule
        const ruleExport: ExportRule = {
            type: 'rule',
            uuid: rule.uuid,
            scenario: rule.scenario,
            request_model: rule.request_model,
            response_model: rule.response_model,
            description: rule.description,
            services: rule.services || [],
            active: rule.active,
            smart_enabled: rule.smart_enabled,
            smart_routing: rule.smart_routing || [],
        };
        lines.push(JSON.stringify(ruleExport));

        // Subsequent lines: Providers
        for (const provider of providersData) {
            const providerExport: ExportProvider = {
                type: 'provider',
                uuid: provider.uuid,
                name: provider.name,
                api_base: provider.api_base,
                api_style: provider.api_style,
                auth_type: provider.auth_type || 'api_key',
                token: provider.token,
                oauth_detail: provider.oauth_detail,
                enabled: provider.enabled,
                proxy_url: provider.proxy_url,
                timeout: provider.timeout,
                tags: provider.tags,
                models: provider.models,
            };
            lines.push(JSON.stringify(providerExport));
        }

        // Create download
        downloadJsonlFile(lines.join('\n'), `${rule.request_model || 'rule'}-${rule.scenario}.jsonl`);
        onNotification('Rule with API keys exported successfully!', 'success');
    } catch (error) {
        console.error('Error exporting rule:', error);
        onNotification('Failed to export rule', 'error');
    }
}

function downloadJsonlFile(content: string, filename: string): void {
    const blob = new Blob([content], { type: 'application/jsonl' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}
