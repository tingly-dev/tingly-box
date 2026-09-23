// Editor-state builders and payload construction for the Guardrails policy
// editor. Pure module functions shared by RulesPage and its editor dialog.
import {
    DEFAULT_GROUP_ID,
    type EditorListField,
    type EditorState,
    type GuardrailsPolicy,
    type OversizedListField,
    type PreparedEditorState,
} from './types';
import { effectiveListValues, prepareListField, splitLines } from './listField';

export const normalizeGroup = (value?: string) => value?.trim() || DEFAULT_GROUP_ID;

export const normalizePolicyGroups = (values?: string[]) => {
    const seen = new Set<string>();
    const out: string[] = [];
    [...(Array.isArray(values) ? values : [])].forEach((value) => {
        const next = normalizeGroup(value);
        if (!next || seen.has(next)) {
            return;
        }
        seen.add(next);
        out.push(next);
    });
    return out;
};

export const ensureDefaultGroupMembership = (values?: string[]) => {
    const groups = normalizePolicyGroups(values);
    if (groups.includes(DEFAULT_GROUP_ID)) {
        return groups;
    }
    return [DEFAULT_GROUP_ID, ...groups];
};

// Generic slug-unique-id loop (previously duplicated as generatePolicyId /
// generateGroupId across RulesPage and GroupsPage).
export const uniqueIdFromName = (name: string, existingIds: Iterable<string>, fallbackBase: string) => {
    const normalizedName = name
        .toLowerCase()
        .trim()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-+|-+$/g, '');
    const baseId = normalizedName || fallbackBase;
    const takenIds = new Set(existingIds);

    let candidate = baseId;
    let suffix = 2;
    while (takenIds.has(candidate)) {
        candidate = `${baseId}-${suffix}`;
        suffix += 1;
    }
    return candidate;
};

export const generatePolicyId = (
    name: string,
    kind: EditorState['kind'],
    existingIds: Iterable<string>,
    currentId?: string
) => {
    const fallbackBase =
        kind === 'resource_access'
            ? 'resource-policy'
            : kind === 'command_execution'
              ? 'command-policy'
              : kind === 'content'
                ? 'content-policy'
                : 'policy';
    const takenIds = Array.from(existingIds).filter((policyId) => policyId && policyId !== currentId);
    return uniqueIdFromName(name, takenIds, fallbackBase);
};

export const sanitizeVerdictForKind = (kind: EditorState['kind'], verdict?: string) => {
    if (verdict === 'mask') {
        return 'block';
    }
    return verdict || 'block';
};

export const applyKindDefaults = (
    kind: 'resource_access' | 'command_execution' | 'content',
    current: EditorState,
    options: { isNewPolicy: boolean; takenIds: Iterable<string> }
): EditorState => {
    const nextCommandAction =
        current.actions.includes('install')
            ? ['install']
            : current.actions.includes('execute')
              ? ['execute']
              : ['execute'];
    return {
        ...current,
        kind,
        name: current.name,
        id:
            options.isNewPolicy && current.name.trim()
                ? generatePolicyId(current.name, kind, options.takenIds)
                : current.id,
        verdict: sanitizeVerdictForKind(kind, current.verdict),
        toolNames: kind === 'content' ? '' : current.toolNames,
        actions:
            kind === 'resource_access'
                ? current.actions.length > 0
                    ? current.actions.filter((action) => action !== 'execute')
                    : ['read']
                : kind === 'command_execution'
                  ? nextCommandAction
                  : [],
        commandTerms: kind === 'command_execution' ? current.commandTerms : '',
        patterns: kind === 'content' ? current.patterns : '',
    };
};

export const buildSuggestedReason = (state: EditorState) => {
    if (state.kind === 'command_execution') {
        if (state.actions.includes('install')) {
            const commandTerms = splitLines(state.commandTerms);
            if (commandTerms.length > 0) {
                return `This policy blocks install commands matching ${commandTerms.join(', ')}.`;
            }
            const resources = splitLines(state.resources);
            if (resources.length > 0) {
                return `This policy blocks install commands that touch ${resources.join(', ')}.`;
            }
            const tools = splitLines(state.toolNames);
            if (tools.length > 0) {
                return `This policy blocks install commands executed through ${tools.join(', ')}.`;
            }
            return 'This policy blocks install commands.';
        }
        const commandTerms = splitLines(state.commandTerms);
        if (commandTerms.length > 0) {
            return `This policy blocks execution of commands matching ${commandTerms.join(', ')}.`;
        }
        const resources = splitLines(state.resources);
        if (resources.length > 0) {
            return `This policy blocks execution of commands that touch ${resources.join(', ')}.`;
        }
        const tools = splitLines(state.toolNames);
        if (tools.length > 0) {
            return `This policy blocks execution through ${tools.join(', ')}.`;
        }
    }
    if (state.kind === 'resource_access') {
        const actions = state.actions.length > 0 ? state.actions.join(', ') : 'access';
        const resources = splitLines(state.resources);
        const resourceLabel = resources.length > 0 ? resources.join(', ') : 'protected resources';
        return `This policy blocks attempts to ${actions} ${resourceLabel}.`;
    }
    const patterns = splitLines(state.patterns);
    if (patterns.length === 0) {
        return 'This policy blocks prohibited content.';
    }
    return `This policy blocks content matching ${patterns.slice(0, 2).join(', ')}.`;
};

export const buildPolicyPayload = (
    state: EditorState,
    oversized: Partial<Record<EditorListField, OversizedListField>> | undefined
) => {
    const commandActions = state.actions.includes('install') ? ['install'] : ['execute'];
    const operationMatch = {
        tool_names: effectiveListValues(oversized, 'toolNames', state.toolNames),
        actions: {
            include:
                state.kind === 'command_execution'
                    ? commandActions
                    : state.actions.filter((action) => action !== 'execute'),
        },
        terms: state.kind === 'command_execution' ? effectiveListValues(oversized, 'commandTerms', state.commandTerms) : [],
        resources: {
            type: 'path',
            mode: state.resourceMode,
            values: effectiveListValues(oversized, 'resources', state.resources),
        },
    };
    const payload = {
        id: state.id,
        name: state.name,
        groups: normalizePolicyGroups(state.groups),
        kind: state.kind,
        enabled: state.enabled,
        scope: {
            scenarios: state.scenarios,
        },
        verdict: state.verdict,
        reason: state.reason,
        match:
            state.kind === 'content'
                ? {
                      patterns: effectiveListValues(oversized, 'patterns', state.patterns),
                      pattern_mode: state.patternMode,
                      case_sensitive: state.caseSensitive,
                  }
                : operationMatch,
    };
    return payload;
};

export const buildBuiltinPayload = (builtin: GuardrailsPolicy, enabled: boolean): GuardrailsPolicy => ({
    id: builtin.id,
    name: builtin.name,
    groups: ensureDefaultGroupMembership(builtin.groups),
    kind: builtin.kind,
    enabled,
    scope: builtin.scope || { scenarios: [] },
    match: builtin.match || {},
    verdict: builtin.verdict || 'block',
    reason: builtin.reason || '',
});

export const makeEditorState = (policy?: GuardrailsPolicy, scenarioOptions: string[] = []): PreparedEditorState => {
    const scenarios =
        policy?.scope?.scenarios && policy.scope.scenarios.length > 0
            ? policy.scope.scenarios
            : scenarioOptions;
    const toolNamesField = prepareListField(policy?.match?.tool_names);
    const commandTermsField = prepareListField(policy?.match?.terms);
    const resourcesField = prepareListField(policy?.match?.resources?.values);
    const patternsField = prepareListField(policy?.match?.patterns);
    const oversized: Partial<Record<EditorListField, OversizedListField>> = {};
    if (toolNamesField.oversized) oversized.toolNames = toolNamesField.oversized;
    if (commandTermsField.oversized) oversized.commandTerms = commandTermsField.oversized;
    if (resourcesField.oversized) oversized.resources = resourcesField.oversized;
    if (patternsField.oversized) oversized.patterns = patternsField.oversized;
    const nextState: EditorState = {
        id: policy?.id || '',
        name: policy?.name || '',
        groups: normalizePolicyGroups(policy?.groups),
        kind: policy?.kind === 'operation' ? 'resource_access' : policy?.kind || '',
        enabled: policy?.enabled === true,
        verdict: sanitizeVerdictForKind(
            policy?.kind === 'operation' ? 'resource_access' : policy?.kind || '',
            policy?.verdict || 'block'
        ),
        scenarios,
        toolNames: toolNamesField.text,
        actions:
            (policy?.kind === 'command_execution'
                ? policy?.match?.actions?.include?.includes('install')
                    ? ['install']
                    : ['execute']
                : policy?.match?.actions?.include) || [],
        commandTerms: commandTermsField.text,
        resources: resourcesField.text,
        resourceMode: policy?.match?.resources?.mode || 'prefix',
        patterns: patternsField.text,
        patternMode: policy?.match?.pattern_mode || 'substring',
        caseSensitive: !!policy?.match?.case_sensitive,
        reason: policy?.reason || '',
    };
    return { state: nextState, oversized };
};

export const makeEditorStateFromDraft = (
    draft: Partial<EditorState>,
    options: { takenIds: Iterable<string>; scenarioOptions: string[] }
): EditorState => {
    const baseState = makeEditorState(undefined, options.scenarioOptions).state;
    const nextKind = draft.kind || baseState.kind;
    const nextName = draft.name || baseState.name;
    const nextID =
        draft.id ||
        (nextKind && nextName.trim() ? generatePolicyId(nextName, nextKind, options.takenIds) : baseState.id);
    return {
        ...baseState,
        ...draft,
        id: nextID,
        name: nextName,
        groups: normalizePolicyGroups(draft.groups).length > 0 ? normalizePolicyGroups(draft.groups) : baseState.groups,
        kind: nextKind,
        enabled: draft.enabled ?? baseState.enabled,
        verdict: sanitizeVerdictForKind(nextKind, draft.verdict || baseState.verdict),
        scenarios:
            draft.scenarios && draft.scenarios.length > 0
                ? draft.scenarios
                : baseState.scenarios,
        toolNames: draft.toolNames ?? baseState.toolNames,
        actions: draft.actions ?? baseState.actions,
        commandTerms: draft.commandTerms ?? baseState.commandTerms,
        resources: draft.resources ?? baseState.resources,
        resourceMode: draft.resourceMode || baseState.resourceMode,
        patterns: draft.patterns ?? baseState.patterns,
        patternMode: draft.patternMode || baseState.patternMode,
        caseSensitive: draft.caseSensitive ?? baseState.caseSensitive,
        reason: draft.reason ?? baseState.reason,
    };
};

// Row selections the open paths seed the dialog list editors with: first row
// focused when the field has content, none otherwise.
export const initialRowSelections = (state: EditorState) => ({
    resourceRow: splitLines(state.resources).length > 0 ? 0 : -1,
    commandTermRow: splitLines(state.commandTerms).length > 0 ? 0 : -1,
    patternRow: splitLines(state.patterns).length > 0 ? 0 : -1,
});
