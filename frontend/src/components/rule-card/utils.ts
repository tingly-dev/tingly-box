import { v4 as uuidv4 } from 'uuid';
import { api } from '@/services/api';
import type { SmartRouting, ConfigProvider, Rule, ConfigRecord, RuleFlags } from '@/components/RoutingGraphTypes';

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Checks if a model name is a wildcard that matches any model
 */
export function isWildcardModelName(modelName: string): boolean {
    return modelName === '*' || modelName === '[any]';
}

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
 * NOTE: Also ensures smart routing rules have UUIDs (backend may not preserve them)
 */
export function normalizeSmartRoutingServices(smartRouting: SmartRouting[]): SmartRouting[] {
    return smartRouting.map((routing) => ({
        ...routing,
        // Ensure the routing itself has a UUID (backend might not preserve it)
        uuid: routing.uuid || uuidv4(),
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
        flags: {
            cursorCompat: rule.flags?.cursor_compat || false,
            cursorCompatAuto: rule.flags?.cursor_compat_auto || false,
        },
        smartEnabled: rule.smart_enabled || false,
        smartRouting: smartRouting,
    };
}

/**
 * Creates a deep copy of a SmartRouting object
 * NOTE: Deep clone is critical to prevent mutation of source data when editing
 */
export function cloneSmartRouting(smartRouting: SmartRouting): SmartRouting {
    return {
        uuid: smartRouting.uuid,
        description: smartRouting.description,
        // Deep clone ops to prevent mutation of nested objects (especially meta)
        ops: smartRouting.ops.map((op) => ({
            uuid: op.uuid,
            position: op.position,
            operation: op.operation,
            value: op.value,
            meta: op.meta ? { ...op.meta } : undefined,
        })),
        // Deep clone services to prevent mutation
        services: smartRouting.services.map((service) => ({
            uuid: service.uuid,
            provider: service.provider,
            model: service.model,
            isManualInput: service.isManualInput,
            weight: service.weight,
            active: service.active,
            time_window: service.time_window,
        })),
    };
}

/**
 * Creates a new empty SmartRouting object
 */
export function createEmptySmartRouting(): SmartRouting {
    return {
        uuid: uuidv4(),  // Use uuid library instead of crypto.randomUUID() for better compatibility
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
    flags?: {
        cursor_compat?: boolean;
        cursor_compat_auto?: boolean;
    };
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

// ============================================================================
// Rule flag helpers
// ============================================================================

const TRUE_VALUES = new Set(['true', '1', 'yes', 'on']);
const FALSE_VALUES = new Set(['false', '0', 'no', 'off']);

export function formatRuleFlags(flags?: RuleFlags): string {
    if (!flags) return '';
    const entries: string[] = [];
    if (flags.cursorCompat) entries.push('cursor_compat=true');
    if (flags.cursorCompatAuto) entries.push('cursor_compat_auto=true');
    return entries.join(',');
}

export function parseRuleFlags(input: string): { flags: RuleFlags; error?: string } {
    const flags: RuleFlags = {
        cursorCompat: false,
        cursorCompatAuto: false,
    };

    const trimmed = input.trim();
    if (!trimmed) {
        return { flags };
    }

    const parts = trimmed.split(',').map((part) => part.trim()).filter(Boolean);
    for (const part of parts) {
        const [rawKey, rawValue] = part.split('=').map((chunk) => chunk.trim());
        if (!rawKey || rawValue === undefined || rawValue === '') {
            return { flags, error: `Invalid flag format: "${part}". Use key=value.` };
        }

        const valueLower = rawValue.toLowerCase();
        let parsedValue: boolean;
        if (TRUE_VALUES.has(valueLower)) {
            parsedValue = true;
        } else if (FALSE_VALUES.has(valueLower)) {
            parsedValue = false;
        } else {
            return { flags, error: `Invalid value for "${rawKey}": "${rawValue}". Use true/false.` };
        }

        switch (rawKey) {
            case 'cursor_compat':
                flags.cursorCompat = parsedValue;
                break;
            case 'cursor_compat_auto':
                flags.cursorCompatAuto = parsedValue;
                break;
            default:
                return { flags, error: `Unknown flag "${rawKey}".` };
        }
    }

    return { flags };
}

// Generic export handler
async function exportData(
    jsonlContent: string,
    format: ExportFormat,
    filename: string,
    notificationMsg: string,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    const content = format === 'jsonl' ? jsonlContent : `${BASE64_PREFIX}:${CURRENT_VERSION}:${btoa(jsonlContent)}`;
    const extension = format === 'jsonl' ? 'jsonl' : 'txt';
    const mimeType = format === 'jsonl' ? 'application/jsonl' : 'text/plain';

    downloadFile(content, `${filename}.${extension}`, mimeType);
    onNotification(notificationMsg, 'success');
}

// Generic clipboard export handler
async function exportToClipboard(
    jsonlContent: string,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    const base64Content = `${BASE64_PREFIX}:${CURRENT_VERSION}:${btoa(jsonlContent)}`;
    await copyToClipboard(base64Content);
    onNotification('Base64 export copied to clipboard! You can now paste it anywhere.', 'success');
}

// Generic JSONL clipboard export handler
async function exportJsonlToClipboard(
    jsonlContent: string,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    await copyToClipboard(jsonlContent);
    onNotification('JSONL export copied to clipboard! You can now paste it anywhere.', 'success');
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
        const jsonlContent = await buildJsonlExport(rule);
        const filename = `${rule.request_model || 'rule'}-${rule.scenario}`;
        const message = format === 'jsonl'
            ? 'Rule with API keys exported successfully!'
            : 'Rule exported as Base64! You can copy and share this file.';
        await exportData(jsonlContent, format, filename, message, onNotification);
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
        const jsonlContent = await buildJsonlExport(rule);
        await exportToClipboard(jsonlContent, onNotification);
    } catch (error) {
        console.error('Error exporting rule to clipboard:', error);
        onNotification('Failed to copy to clipboard', 'error');
    }
}

/**
 * Exports a rule as JSONL and copies to clipboard
 */
export async function exportRuleAsJsonlToClipboard(
    rule: Rule,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    try {
        const jsonlContent = await buildJsonlExport(rule);
        await exportJsonlToClipboard(jsonlContent, onNotification);
    } catch (error) {
        console.error('Error exporting rule to clipboard:', error);
        onNotification('Failed to copy to clipboard', 'error');
    }
}

/**
 * Exports a single provider to the specified format
 */
export async function exportProvider(
    provider: any,
    format: ExportFormat,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    try {
        const jsonlContent = buildProviderJsonlExport(provider);
        const filename = `${provider.name || 'provider'}-${provider.api_style}`;
        const message = format === 'jsonl'
            ? 'Provider exported successfully!'
            : 'Provider exported as Base64! You can copy and share this file.';
        await exportData(jsonlContent, format, filename, message, onNotification);
    } catch (error) {
        console.error('Error exporting provider:', error);
        onNotification('Failed to export provider', 'error');
    }
}

/**
 * Exports a provider as Base64 and copies to clipboard
 */
export async function exportProviderAsBase64ToClipboard(
    provider: any,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    try {
        const jsonlContent = buildProviderJsonlExport(provider);
        await exportToClipboard(jsonlContent, onNotification);
    } catch (error) {
        console.error('Error exporting provider to clipboard:', error);
        onNotification('Failed to copy to clipboard', 'error');
    }
}

/**
 * Exports a provider as JSONL and copies to clipboard
 */
export async function exportProviderAsJsonlToClipboard(
    provider: any,
    onNotification: (message: string, severity: 'success' | 'error') => void
): Promise<void> {
    try {
        const jsonlContent = buildProviderJsonlExport(provider);
        await exportJsonlToClipboard(jsonlContent, onNotification);
    } catch (error) {
        console.error('Error exporting provider to clipboard:', error);
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
        if (service.provider) providerUuids.add(service.provider);
    });

    // Fetch all providers
    const providersData: any[] = [];
    for (const uuid of providerUuids) {
        try {
            const result = await api.getProvider(uuid);
            if (result.success && result.data) providersData.push(result.data);
        } catch (error) {
            console.error(`Failed to fetch provider ${uuid}:`, error);
        }
    }

    return buildJsonlLines([
        createMetadataLine(),
        createRuleLine(rule),
        ...providersData.map(createProviderLine)
    ]);
}

/**
 * Builds the JSONL export content for a single provider
 */
function buildProviderJsonlExport(provider: any): string {
    return buildJsonlLines([
        createMetadataLine(),
        createProviderLine(provider)
    ]);
}

// Helper functions for building export lines
function createMetadataLine(): string {
    return JSON.stringify({
        type: 'metadata',
        version: CURRENT_VERSION,
        exported_at: new Date().toISOString()
    } as ExportMetadata);
}

function createRuleLine(rule: Rule): string {
    return JSON.stringify({
        type: 'rule',
        uuid: rule.uuid,
        scenario: rule.scenario,
        request_model: rule.request_model,
        response_model: rule.response_model,
        description: rule.description,
        services: rule.services || [],
        active: rule.active,
        flags: rule.flags,
        smart_enabled: rule.smart_enabled,
        smart_routing: rule.smart_routing || [],
    } as ExportRule);
}

function createProviderLine(provider: any): string {
    return JSON.stringify({
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
    } as ExportProvider);
}

function buildJsonlLines(lines: string[]): string {
    return lines.join('\n');
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

    const [version, payload] = [parts[1], parts[2]];
    if (version !== CURRENT_VERSION) {
        throw new Error(`Unsupported version: ${version} (supported: ${CURRENT_VERSION})`);
    }

    return atob(payload);
}

/**
 * Copies text to clipboard
 */
async function copyToClipboard(text: string): Promise<void> {
    if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
    } else {
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
 * Downloads content as a file
 */
function downloadFile(content: string, filename: string, mimeType: string): void {
    const blob = new Blob([content], { type: mimeType });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}
