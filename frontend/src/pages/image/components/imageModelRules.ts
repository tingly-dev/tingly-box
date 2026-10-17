import type { Rule } from '@/components/RoutingGraphTypes';

// The Playground picks a provider · model, not a rule (.design/image-layout.md
// §4). Every run still goes through an imagegen rule on /tingly/imagegen;
// these helpers map between the two so the rule stays out of sight.

export interface DirectService {
    provider: string;
    model: string;
}

// A rule that sends every request to one provider · model, so the picker can
// show that concrete pair instead of the rule's name. Rules that balance
// across services or route by smart rules have no single answer.
export const directService = (rule: Rule): DirectService | null => {
    if (rule.smart_enabled && (rule.smart_routing?.length ?? 0) > 0) return null;
    const services = (rule.services ?? []).filter((service) => service.active !== false && service.provider && service.model);
    if (services.length !== 1) return null;
    return { provider: services[0].provider, model: services[0].model };
};

export interface ImageModelOption {
    // What a run sends as `model`: the rule's request_model.
    requestModel: string;
    direct: DirectService | null;
}

// Usable rules, one option per request_model, direct ones first so the pairs
// a person picked themselves lead the list.
export const imageModelOptions = (rules: Rule[]): ImageModelOption[] => {
    const seen = new Set<string>();
    const options: ImageModelOption[] = [];
    for (const rule of rules) {
        const requestModel = rule.request_model?.trim();
        if (rule.active === false || !requestModel || seen.has(requestModel)) continue;
        seen.add(requestModel);
        options.push({ requestModel, direct: directService(rule) });
    }
    return [
        ...options.filter((option) => option.direct),
        ...options.filter((option) => !option.direct),
    ];
};

// The active rule that already sends everything to this provider · model, so
// picking the same pair twice reuses it instead of piling up copies.
export const findDirectRule = (rules: Rule[], provider: string, model: string): Rule | undefined =>
    rules.find((rule) => {
        if (rule.active === false || !rule.request_model?.trim()) return false;
        const direct = directService(rule);
        return direct?.provider === provider && direct.model === model;
    });
