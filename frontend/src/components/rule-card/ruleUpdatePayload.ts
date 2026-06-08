import type { ConfigRecord, Rule, RuleFlagsApi, SmartRouting } from '@/components/RoutingGraphTypes';
import { flagsToApi } from './flagHelpers';

export interface RuleUpdateService {
    provider: string;
    model: string;
    weight: number;
    active: boolean;
    time_window: number;
    tier: number;
}

export interface RuleUpdatePayload {
    uuid: string;
    scenario: string;
    request_model: string;
    response_model: string;
    active: boolean;
    description?: string;
    flags: RuleFlagsApi;
    services: RuleUpdateService[];
    smart_enabled: boolean;
    smart_routing: SmartRouting[];
}

/**
 * Builds the full rule-update request body from a ConfigRecord.
 *
 * The UpdateRule endpoint uses full-replace (PUT) semantics: every field the
 * backend persists must be present here, otherwise it gets cleared. In
 * particular `flags` MUST always be included — omitting it on the model-select
 * path was the cause of the "switching a rule's model wiped its flags" bug.
 * Centralizing the payload here keeps every call site (model-select dialog,
 * rule-card auto-save) in sync so a field can't silently go missing on one path.
 */
export function buildRuleUpdatePayload(
    rule: Pick<Rule, 'uuid' | 'scenario'>,
    config: ConfigRecord,
): RuleUpdatePayload {
    return {
        uuid: rule.uuid,
        scenario: rule.scenario,
        request_model: config.requestModel,
        response_model: config.responseModel,
        active: config.active,
        description: config.description,
        flags: flagsToApi(config.flags),
        services: config.providers
            .filter((p) => p.provider && p.model)
            .map((provider) => ({
                provider: provider.provider,
                model: provider.model,
                weight: provider.weight || 0,
                active: provider.active !== undefined ? provider.active : true,
                time_window: provider.time_window || 0,
                tier: provider.tier ?? 0,
            })),
        smart_enabled: config.smartEnabled || false,
        smart_routing: config.smartRouting || [],
    };
}
