// Presentation helpers for rendering guardrail policies (effective state,
// summaries). Pure module functions shared by RulesPage and GroupsPage.
import { DEFAULT_GROUP_ID, type DisplayPolicy, type GuardrailsPolicy, type PolicyGroup } from './types';
import { ensureDefaultGroupMembership, normalizePolicyGroups } from './editorState';
import { summarizeValues } from './listField';

export const getEffectivePolicyState = (policy: GuardrailsPolicy, groupsById: Map<string, PolicyGroup>) => {
    const policyGroups = ensureDefaultGroupMembership(policy.groups);
    const noActiveGroup = policyGroups.length === 0 || policyGroups.every((groupID) => groupsById.get(groupID)?.enabled !== true);
    return {
        inheritedDisabled: noActiveGroup,
        visibleEnabled: policy.enabled === true && !noActiveGroup,
    };
};

export const policyNeedsEnableWithDefault = (policy: DisplayPolicy, policies: GuardrailsPolicy[]) => {
    const installedPolicy = policies.find((item) => item.id === policy.id);
    if (!installedPolicy) {
        return policy.isBuiltin;
    }
    const groups = normalizePolicyGroups(installedPolicy.groups);
    return installedPolicy.enabled !== true || !groups.includes(DEFAULT_GROUP_ID);
};

export const policyNeedsDisable = (policy: DisplayPolicy, policies: GuardrailsPolicy[]) => {
    const installedPolicy = policies.find((item) => item.id === policy.id);
    return Boolean(installedPolicy && installedPolicy.enabled === true);
};

export const buildPolicySummary = (policy: DisplayPolicy) => {
    if (policy.kind === 'command_execution') {
        const actionList = policy.match?.actions?.include || [];
        const action = actionList.includes('install') ? 'install' : 'execute';
        const resources = summarizeValues(policy.match?.resources?.values, '');
        const toolNames = summarizeValues(policy.match?.tool_names, '');
        if (action === 'install') {
            const terms = summarizeValues(policy.match?.terms, 'any install target');
            return [toolNames, 'install', terms, resources && resources !== 'none' ? resources : '']
                .filter(Boolean)
                .join(' · ');
        }
        const terms = summarizeValues(policy.match?.terms, 'any command');
        return [toolNames, 'execute', terms, resources && resources !== 'none' ? resources : '']
            .filter(Boolean)
            .join(' · ');
    }
    if (policy.kind === 'resource_access' || policy.kind === 'operation') {
        const actions = summarizeValues(policy.match?.actions?.include, 'any action');
        const resources = summarizeValues(policy.match?.resources?.values, 'any resource');
        const toolNames = summarizeValues(policy.match?.tool_names, '');
        return [toolNames, actions, resources].filter(Boolean).join(' · ');
    }
    const patterns = policy.match?.patterns || [];
    if (patterns.length === 0) {
        if (policy.isBuiltin && policy.builtinSummary) {
            return policy.builtinSummary;
        }
        return 'No patterns configured';
    }
    return summarizeValues(patterns, 'No patterns configured');
};

export const buildPolicyScope = (policy: DisplayPolicy) => {
    const scenarios = policy.scope?.scenarios?.join(', ') || '';
    return scenarios;
};
