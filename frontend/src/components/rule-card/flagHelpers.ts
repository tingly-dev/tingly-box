import type { FlagSpec, RuleFlags, RuleFlagsApi, VisionProxyServiceRef } from '@/components/RoutingGraphTypes';

export function snakeToCamel(s: string): string {
    return s.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
}

export function camelToSnake(s: string): string {
    return s.replace(/[A-Z]/g, (c) => `_${c.toLowerCase()}`);
}

export function getFlagValue(flags: RuleFlags | undefined, key: string): unknown {
    if (!flags) return undefined;
    return (flags as Record<string, unknown>)[snakeToCamel(key)];
}

export function setFlagValue(flags: RuleFlags, key: string, value: unknown): RuleFlags {
    return { ...flags, [snakeToCamel(key)]: value };
}

export function flagDefault(spec: FlagSpec): unknown {
    switch (spec.type) {
        case 'bool': return false;
        case 'string': return '';
        case 'int': return 0;
        case 'enum': return spec.options?.[0]?.value ?? '';
        case 'service_ref': return undefined;
    }
}

export function enumInactive(spec: FlagSpec): string {
    return spec.options?.[0]?.value ?? '';
}

export function isFlagActive(spec: FlagSpec, flags: RuleFlags): boolean {
    const value = getFlagValue(flags, spec.key);
    switch (spec.type) {
        case 'bool': return !!value;
        case 'string': return typeof value === 'string' && value !== '';
        case 'int': return typeof value === 'number' && value > 0;
        case 'enum': {
            const inactive = enumInactive(spec);
            return value !== '' && value !== undefined && value !== inactive;
        }
        case 'service_ref': {
            const ref = value as VisionProxyServiceRef | undefined;
            return !!(ref && ref.provider && ref.model);
        }
        default: return false;
    }
}

// Collapse enum inactive sentinel to '' for omitempty on the wire.
export function normalizeEnumForStorage(spec: FlagSpec, value: string): string {
    const inactive = enumInactive(spec);
    return (inactive !== '' && value === inactive) ? '' : value;
}

export function apiToFlags(api: RuleFlagsApi | undefined): RuleFlags {
    if (!api) return {};
    const flags: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(api)) {
        flags[snakeToCamel(key)] = value;
    }
    return flags as RuleFlags;
}

export function flagsToApi(flags: RuleFlags | undefined): RuleFlagsApi {
    if (!flags) return {};
    const api: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(flags)) {
        api[camelToSnake(key)] = value;
    }
    return api as RuleFlagsApi;
}
