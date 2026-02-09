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

export type ExportFormat = 'jsonl' | 'base64';

const BASE64_PREFIX = 'TGB64';
const CURRENT_VERSION = '1.0';

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
 * Exports a rule with its associated providers to the specified format
 */
export async function exportRuleWithProviders(
    rule: Rule,
    format: ExportFormat,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    try {
        // Build JSONL content first
        const jsonlContent = await buildJsonlExport(rule);

        if (format === 'jsonl') {
            // Download as JSONL file
            downloadJsonlFile(jsonlContent, `${rule.request_model || 'rule'}-${rule.scenario}.jsonl`);
            onNotification('Rule with API keys exported successfully!', 'success');
        } else {
            // Convert to Base64 format
            const base64Content = encodeBase64Export(jsonlContent);
            // Download as text file
            downloadTextFile(base64Content, `${rule.request_model || 'rule'}-${rule.scenario}.txt`);
            onNotification('Rule exported as Base64! You can copy and share this file.', 'success');
        }
    } catch (error) {
        console.error('Error exporting rule:', error);
        onNotification('Failed to export rule', 'error');
    }
}

/**
 * Exports a rule as Base64 and copies to clipboard
 */
export async function exportRuleAsBase64ToClipboard(
    rule: Rule,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    try {
        // Build JSONL content
        const jsonlContent = await buildJsonlExport(rule);

        // Convert to Base64 format
        const base64Content = encodeBase64Export(jsonlContent);

        // Copy to clipboard
        await copyToClipboard(base64Content);
        onNotification('Base64 export copied to clipboard! You can now paste it anywhere.', 'success');
    } catch (error) {
        console.error('Error exporting rule to clipboard:', error);
        onNotification('Failed to copy to clipboard', 'error');
    }
}

/**
 * Builds the JSONL export content for a rule
 */
async function buildJsonlExport(rule: Rule): Promise<string> {
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
        version: CURRENT_VERSION,
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

    return lines.join('\n');
}

/**
 * Encodes JSONL content as Base64 export format
 */
function encodeBase64Export(jsonlContent: string): string {
    // Convert to Base64
    const base64 = btoa(jsonlContent);
    // Add prefix
    return `${BASE64_PREFIX}:${CURRENT_VERSION}:${base64}`;
}

/**
 * Decodes Base64 export content back to JSONL
 */
export function decodeBase64Export(base64Content: string): string {
    const trimmed = base64Content.trim();

    if (!trimmed.startsWith(`${BASE64_PREFIX}:`)) {
        throw new Error('Invalid Base64 export format: missing prefix');
    }

    const parts = trimmed.split(':');
    if (parts.length !== 3) {
        throw new Error('Invalid Base64 export format: expected prefix:version:payload');
    }

    const version = parts[1];
    const payload = parts[2];

    if (version !== CURRENT_VERSION) {
        throw new Error(`Unsupported version: ${version} (supported: ${CURRENT_VERSION})`);
    }

    // Decode Base64
    return atob(payload);
}

/**
 * Copies text to clipboard
 */
async function copyToClipboard(text: string): Promise<void> {
    if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
    } else {
        // Fallback for older browsers
        const textArea = document.createElement('textarea');
        textArea.value = text;
        textArea.style.position = 'fixed';
        textArea.style.left = '-999999px';
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();
        try {
            document.execCommand('copy');
        } finally {
            document.body.removeChild(textArea);
        }
    }
}

/**
 * Downloads content as a JSONL file
 */
function downloadJsonlFile(content: string, filename: string): void {
    const blob = new Blob([content], { type: 'application/jsonl' });
    downloadBlob(blob, filename);
}

/**
 * Downloads content as a text file
 */
function downloadTextFile(content: string, filename: string): void {
    const blob = new Blob([content], { type: 'text/plain' });
    downloadBlob(blob, filename);
}

/**
 * Downloads a blob as a file
 */
function downloadBlob(blob: Blob, filename: string): void {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}
