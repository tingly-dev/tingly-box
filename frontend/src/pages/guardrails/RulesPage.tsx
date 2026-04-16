import { useEffect, useMemo, useState } from 'react';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Alert,
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    FormControl,
    FormControlLabel,
    FormHelperText,
    IconButton,
    InputBase,
    InputLabel,
    List,
    ListItem,
    MenuItem,
    Paper,
    Select,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tab,
    Tabs,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {
    Add,
    ArticleOutlined,
    AutoAwesome,
    CheckCircleRounded,
    Code as CodeIcon,
    DeleteOutline,
    ExpandMore,
    HelpOutline,
    LaptopMac,
    Refresh,
    Rule,
    Remove,
    Terminal,
} from '@mui/icons-material';
import { Anthropic, Claude, OpenAI } from '@/components/BrandIcons';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { useLocation, useNavigate } from 'react-router-dom';

type PolicyGroup = {
    id: string;
    name?: string;
    severity?: string;
    enabled?: boolean;
};

type GuardrailsPolicy = {
    id: string;
    name?: string;
    groups?: string[];
    kind: 'resource_access' | 'command_execution' | 'content' | 'operation';
    enabled?: boolean;
    scope?: {
        scenarios?: string[];
    };
    match?: {
        tool_names?: string[];
        actions?: { include?: string[]; exclude?: string[] };
        resources?: { type?: string; mode?: string; values?: string[] };
        terms?: string[];
        credential_refs?: string[];
        patterns?: string[];
        pattern_mode?: string;
        case_sensitive?: boolean;
    };
    verdict?: string;
    reason?: string;
};

type DisplayPolicy = GuardrailsPolicy & {
    isBuiltin?: boolean;
    builtinSummary?: string;
};

type RegistryPolicyEntry = {
    id: string;
    name?: string;
    reason?: string;
    path: string;
};

type EditorState = {
    id: string;
    name: string;
    groups: string[];
    kind: 'resource_access' | 'command_execution' | 'content' | '';
    enabled: boolean;
    verdict: string;
    scenarios: string[];
    toolNames: string;
    actions: string[];
    commandTerms: string;
    resources: string;
    resourceMode: string;
    patterns: string;
    patternMode: string;
    caseSensitive: boolean;
    reason: string;
};

type EditorListField = 'toolNames' | 'commandTerms' | 'resources' | 'patterns';

type OversizedListField = {
    values: string[];
    preview: string[];
    total: number;
};

type PreparedEditorState = {
    state: EditorState;
    oversized: Partial<Record<EditorListField, OversizedListField>>;
};

const MAX_SUMMARY_VALUES = 2;
const MAX_SUMMARY_CHARS = 140;
// Heuristic editability thresholds for large generated guardrail lists.
// These optimize for Rules page responsiveness, not persistence size: once a
// list grows into the hundreds of entries, joining it into one large string,
// repeatedly splitting it on render, and mounting one editable row per value
// starts to make the dialog sluggish. We keep normal hand-authored policies
// fully editable, but switch machine-generated blocklists into preview mode
// before they become expensive to render.
//
// The specific cutoffs are intentionally conservative and were chosen around
// the datasets introduced in this PR:
// - medium generated lists such as the NuGet and RubyGems malicious package
//   blocklists are already well above 400 entries and should use preview mode
// - very large PyPI and npm blocklists always use preview mode
// - smaller policies remain inline-editable
const MAX_INLINE_LIST_ITEMS = 400;
// Secondary guardrail for cases where the item count is moderate but the total
// joined text is still large enough to hurt UI responsiveness.
const MAX_INLINE_LIST_CHARS = 16000;
const OVERSIZED_LIST_PREVIEW_ITEMS = 25;

const resourceAccessActionOptions = [
    {
        value: 'read',
        label: 'Read',
        description: 'Inspect or list files, directories, and other protected paths.',
    },
    {
        value: 'write',
        label: 'Write',
        description: 'Create or modify files, directories, or configuration content.',
    },
    {
        value: 'delete',
        label: 'Delete',
        description: 'Remove files, directories, or other protected resources.',
    },
    {
        value: 'network',
        label: 'Network',
        description: 'Fetch from or send data to remote endpoints.',
    },
] as const;

const commandExecutionActionOptions = [
    {
        value: 'execute',
        label: 'Execute',
        description: 'Match explicit command patterns such as rm -rf, curl | sh, or python -c.',
    },
    {
        value: 'install',
        label: 'Install',
        description: 'Match normalized package, tool, or extension install commands such as npm install or pip install.',
    },
] as const;

const DEFAULT_GROUP_ID = 'default';
const PENDING_REGISTRY_INSTALLS_STORAGE_KEY = 'guardrails.pendingRegistryInstalls';

const readPendingRegistryInstallIds = (): Set<string> => {
    if (typeof window === 'undefined') {
        return new Set<string>();
    }
    try {
        const raw = window.sessionStorage.getItem(PENDING_REGISTRY_INSTALLS_STORAGE_KEY);
        if (!raw) {
            return new Set<string>();
        }
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed)) {
            return new Set<string>();
        }
        return new Set<string>(
            parsed
                .map((value) => (typeof value === 'string' ? value.trim() : ''))
                .filter(Boolean)
        );
    } catch {
        return new Set<string>();
    }
};

const writePendingRegistryInstallIds = (pending: Set<string>) => {
    if (typeof window === 'undefined') {
        return;
    }
    try {
        if (pending.size === 0) {
            window.sessionStorage.removeItem(PENDING_REGISTRY_INSTALLS_STORAGE_KEY);
            return;
        }
        window.sessionStorage.setItem(PENDING_REGISTRY_INSTALLS_STORAGE_KEY, JSON.stringify(Array.from(pending)));
    } catch {
    }
};

const addPendingRegistryInstallId = (policyId: string) => {
    const next = readPendingRegistryInstallIds();
    next.add(policyId);
    writePendingRegistryInstallIds(next);
};

const removePendingRegistryInstallId = (policyId: string) => {
    const next = readPendingRegistryInstallIds();
    next.delete(policyId);
    writePendingRegistryInstallIds(next);
};

const GuardrailsRulesPage = () => {
    const location = useLocation();
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [supportedScenarios, setSupportedScenarios] = useState<string[]>([]);
    const [groups, setGroups] = useState<PolicyGroup[]>([]);
    const [policies, setPolicies] = useState<GuardrailsPolicy[]>([]);
    const [builtins, setBuiltins] = useState<GuardrailsPolicy[]>([]);
    const [registryURL, setRegistryURL] = useState('');
    const [registryPolicies, setRegistryPolicies] = useState<RegistryPolicyEntry[]>([]);
    const [registryLoading, setRegistryLoading] = useState(true);
    const [registryLoadError, setRegistryLoadError] = useState<string | null>(null);
    const [pendingRegistryInstallIds, setPendingRegistryInstallIds] = useState<Set<string>>(() => readPendingRegistryInstallIds());
    const [pendingPolicyId, setPendingPolicyId] = useState<string | null>(null);
    const [pendingSave, setPendingSave] = useState(false);
    const [pendingBulkPolicyAction, setPendingBulkPolicyAction] = useState<'enable' | 'disable' | null>(null);
    const [selectedPolicyId, setSelectedPolicyId] = useState<string | null>(null);
    const [isNewPolicy, setIsNewPolicy] = useState(false);
    const [editorOpen, setEditorOpen] = useState(false);
    const [editorSnapshot, setEditorSnapshot] = useState('');
    const [confirmCloseOpen, setConfirmCloseOpen] = useState(false);
    const [deletePolicyId, setDeletePolicyId] = useState<string | null>(null);
    const [initializingDefaultGroup, setInitializingDefaultGroup] = useState(false);
    const [advancedOpen, setAdvancedOpen] = useState(false);
    const [selectedPolicyTab, setSelectedPolicyTab] = useState<'resource_access' | 'command_execution' | 'content'>(
        'resource_access'
    );
    const [selectedResourceRow, setSelectedResourceRow] = useState(-1);
    const [selectedCommandTermRow, setSelectedCommandTermRow] = useState(-1);
    const [selectedPatternRow, setSelectedPatternRow] = useState(-1);
    const [oversizedListFields, setOversizedListFields] = useState<Partial<Record<EditorListField, OversizedListField>>>({});
    const [editorState, setEditorState] = useState<EditorState>({
        id: '',
        name: '',
        groups: [DEFAULT_GROUP_ID],
        kind: '',
        enabled: false,
        verdict: 'block',
        scenarios: [],
        toolNames: '',
        actions: [],
        commandTerms: '',
        resources: '',
        resourceMode: 'prefix',
        patterns: '',
        patternMode: 'substring',
        caseSensitive: false,
        reason: '',
    });

    const splitLines = (value: string) =>
        value
            .split('\n')
            .map((item) => item.trim())
            .filter(Boolean);

    const textListRows = (value: string) => {
        const rows = value.split('\n');
        if (rows.length === 0) {
            return [''];
        }
        if (rows.length === 1 && rows[0] === '') {
            return [''];
        }
        return rows;
    };

    const joinLines = (values?: string[]) => (Array.isArray(values) ? values.join('\n') : '');
    const isOversizedList = (values?: string[]) => {
        if (!Array.isArray(values) || values.length === 0) {
            return false;
        }
        if (values.length > MAX_INLINE_LIST_ITEMS) {
            return true;
        }
        let chars = 0;
        for (const value of values) {
            chars += value.length + 1;
            if (chars > MAX_INLINE_LIST_CHARS) {
                return true;
            }
        }
        return false;
    };
    const prepareListField = (values?: string[]) => {
        const list = Array.isArray(values) ? values.filter(Boolean) : [];
        if (!isOversizedList(list)) {
            return { text: joinLines(list), oversized: undefined };
        }
        return {
            text: joinLines(list.slice(0, OVERSIZED_LIST_PREVIEW_ITEMS)),
            oversized: {
                values: list,
                preview: list.slice(0, OVERSIZED_LIST_PREVIEW_ITEMS),
                total: list.length,
            } satisfies OversizedListField,
        };
    };
    const summarizeValues = (values?: string[], emptyLabel = 'none') => {
        const list = Array.isArray(values) ? values.filter(Boolean) : [];
        if (list.length === 0) {
            return emptyLabel;
        }
        const preview = list.slice(0, MAX_SUMMARY_VALUES).join(', ');
        const suffix = list.length > MAX_SUMMARY_VALUES ? ` (+${list.length - MAX_SUMMARY_VALUES} more)` : '';
        const text = `${preview}${suffix}`;
        if (text.length <= MAX_SUMMARY_CHARS) {
            return text;
        }
        return `${text.slice(0, MAX_SUMMARY_CHARS - 1)}…`;
    };
    const effectiveListValues = (field: EditorListField, rawValue: string) => {
        const oversized = oversizedListFields[field];
        if (!oversized) {
            return splitLines(rawValue);
        }
        const rawRows = textListRows(rawValue);
        const editablePrefixCount = Math.max(0, rawRows.length - oversized.preview.length);
        const additions = rawRows
            .slice(0, editablePrefixCount)
            .map((item) => item.trim())
            .filter(Boolean);
        return [...additions, ...oversized.values];
    };
    const normalizeGroup = (value?: string) => value?.trim() || DEFAULT_GROUP_ID;
    const normalizePolicyGroups = (values?: string[]) => {
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
    const ensureDefaultGroupMembership = (values?: string[]) => {
        const groups = normalizePolicyGroups(values);
        if (groups.includes(DEFAULT_GROUP_ID)) {
            return groups;
        }
        return [DEFAULT_GROUP_ID, ...groups];
    };

    const toggleValue = (values: string[], value: string) => {
        if (values.includes(value)) {
            return values.filter((item) => item !== value);
        }
        return [...values, value];
    };

    const updateTextListValue = (value: string, index: number, nextItem: string) => {
        const items = textListRows(value);
        while (items.length <= index) {
            items.push('');
        }
        items[index] = nextItem;
        return items.join('\n');
    };

    const appendTextListValue = (value: string) => {
        const items = textListRows(value);
        items.push('');
        return items.join('\n');
    };

    const removeTextListValue = (value: string, index: number) => {
        const items = textListRows(value);
        if (index < 0 || index >= items.length) {
            return value;
        }
        items.splice(index, 1);
        if (items.length === 0) {
            return '';
        }
        return items.join('\n');
    };

    const buildBuiltinPayload = (builtin: GuardrailsPolicy, enabled: boolean): GuardrailsPolicy => ({
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

    const isEditorDirty = useMemo(() => {
        if (!editorSnapshot) {
            return false;
        }
        return JSON.stringify(editorState) !== editorSnapshot;
    }, [editorState, editorSnapshot]);

    const scenarioOptions = useMemo(() => supportedScenarios.filter(Boolean), [supportedScenarios]);
    const groupOptions = useMemo(
        () => groups
            .slice()
            .sort((a, b) => {
                if (a.id === DEFAULT_GROUP_ID) return -1;
                if (b.id === DEFAULT_GROUP_ID) return 1;
                return (a.name || a.id).localeCompare(b.name || b.id);
            })
            .map((group) => ({ value: group.id, label: group.name || group.id })),
        [groups]
    );
    const groupsById = useMemo(
        () => new Map(groups.map((group) => [group.id, group])),
        [groups]
    );
    const builtinMap = useMemo(() => new Map(builtins.map((builtin) => [builtin.id, builtin])), [builtins]);
    const installedPolicyIds = useMemo(() => new Set(policies.map((policy) => policy.id)), [policies]);
    const displayPolicies = useMemo(() => {
        const merged: DisplayPolicy[] = policies.map((policy) => ({
            ...policy,
            isBuiltin: builtinMap.has(policy.id),
            builtinSummary: builtinMap.get(policy.id)?.reason,
        }));
        for (const builtin of builtins) {
            if (installedPolicyIds.has(builtin.id)) {
                continue;
            }
            merged.push({
                id: builtin.id,
                name: builtin.name,
                groups: ensureDefaultGroupMembership(builtin.groups),
                kind: builtin.kind,
                enabled: false,
                scope: builtin.scope,
                match: builtin.match,
                verdict: builtin.verdict || 'block',
                reason: builtin.reason || '',
                isBuiltin: true,
                builtinSummary: builtin.reason,
            });
        }
        const rank = (policy: DisplayPolicy) => (policy.enabled === true ? 0 : 1);
        merged.sort((a, b) => {
            const rankDiff = rank(a) - rank(b);
            if (rankDiff !== 0) return rankDiff;
            return (a.name || a.id).localeCompare(b.name || b.id);
        });
        return merged;
    }, [builtinMap, builtins, installedPolicyIds, policies]);
    const downloadablePolicies = useMemo(
        () => registryPolicies.slice().sort((a, b) => a.id.localeCompare(b.id)),
        [registryPolicies]
    );
    const resourceAccessPolicies = useMemo(
        () => displayPolicies.filter((policy) => policy.kind === 'resource_access' || policy.kind === 'operation'),
        [displayPolicies]
    );
    const commandExecutionPolicies = useMemo(
        () => displayPolicies.filter((policy) => policy.kind === 'command_execution'),
        [displayPolicies]
    );
    const contentPolicies = useMemo(
        () => displayPolicies.filter((policy) => policy.kind === 'content'),
        [displayPolicies]
    );
    const selectedTabPolicies = useMemo(() => {
        if (selectedPolicyTab === 'resource_access') {
            return resourceAccessPolicies;
        }
        if (selectedPolicyTab === 'command_execution') {
            return commandExecutionPolicies;
        }
        return contentPolicies;
    }, [commandExecutionPolicies, contentPolicies, resourceAccessPolicies, selectedPolicyTab]);
    const selectedTabLabel = useMemo(() => {
        if (selectedPolicyTab === 'resource_access') {
            return 'Resource Access';
        }
        if (selectedPolicyTab === 'command_execution') {
            return 'Command Execution';
        }
        return 'Privacy';
    }, [selectedPolicyTab]);

    const getEffectivePolicyState = (policy: GuardrailsPolicy) => {
        const policyGroups = ensureDefaultGroupMembership(policy.groups);
        const noActiveGroup = policyGroups.length === 0 || policyGroups.every((groupID) => groupsById.get(groupID)?.enabled !== true);
        return {
            inheritedDisabled: noActiveGroup,
            visibleEnabled: policy.enabled === true && !noActiveGroup,
        };
    };
    const policyNeedsEnableWithDefault = (policy: DisplayPolicy) => {
        const installedPolicy = policies.find((item) => item.id === policy.id);
        if (!installedPolicy) {
            return policy.isBuiltin;
        }
        const groups = normalizePolicyGroups(installedPolicy.groups);
        return installedPolicy.enabled !== true || !groups.includes(DEFAULT_GROUP_ID);
    };
    const policyNeedsDisable = (policy: DisplayPolicy) => {
        const installedPolicy = policies.find((item) => item.id === policy.id);
        return Boolean(installedPolicy && installedPolicy.enabled === true);
    };

    const generatePolicyId = (name: string, kind: EditorState['kind'], currentId?: string) => {
        const normalizedName = name
            .toLowerCase()
            .trim()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '');
        const fallbackBase =
            kind === 'resource_access'
                ? 'resource-policy'
                : kind === 'command_execution'
                  ? 'command-policy'
                  : kind === 'content'
                    ? 'content-policy'
                    : 'policy';
        const baseId = normalizedName || fallbackBase;
        const existingIds = new Set(
            policies.map((policy) => policy.id).filter((policyId) => policyId && policyId !== currentId)
        );

        let candidate = baseId;
        let suffix = 2;
        while (existingIds.has(candidate)) {
            candidate = `${baseId}-${suffix}`;
            suffix += 1;
        }
        return candidate;
    };

    const applyKindDefaults = (kind: 'resource_access' | 'command_execution' | 'content', current: EditorState): EditorState => {
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
            id: isNewPolicy && current.name.trim() ? generatePolicyId(current.name, kind) : current.id,
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

    const buildSuggestedReason = (state: EditorState) => {
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

    const buildPolicySummary = (policy: DisplayPolicy) => {
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

    const buildPolicyScope = (policy: DisplayPolicy) => {
        const scenarios = policy.scope?.scenarios?.join(', ') || '';
        return scenarios;
    };

    const sanitizeVerdictForKind = (kind: EditorState['kind'], verdict?: string) => {
        if (verdict === 'mask') {
            return 'block';
        }
        return verdict || 'block';
    };

    // MUI restores focus to the trigger after a dialog closes. Blur it so toolbar buttons
    // do not keep the white focus overlay after closing policy/group dialogs.
    const blurActiveElement = () => {
        const active = document.activeElement;
        if (active instanceof HTMLElement) {
            active.blur();
        }
    };

    const makeEditorState = (policy?: GuardrailsPolicy): PreparedEditorState => {
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

    const makeEditorStateFromDraft = (draft: Partial<EditorState>): EditorState => {
        const baseState = makeEditorState().state;
        const nextKind = draft.kind || baseState.kind;
        const nextName = draft.name || baseState.name;
        const nextID =
            draft.id ||
            (nextKind && nextName.trim() ? generatePolicyId(nextName, nextKind) : baseState.id);
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

    const loadPolicies = async (silent = false) => {
        try {
            if (!silent) {
                setLoading(true);
            }
            const [guardrailsConfig, builtinResponse] = await Promise.allSettled([
                api.getGuardrailsConfig(),
                api.getGuardrailsBuiltins(),
            ]);

            if (guardrailsConfig.status !== 'fulfilled' || builtinResponse.status !== 'fulfilled') {
                throw (guardrailsConfig.status === 'rejected' ? guardrailsConfig.reason : builtinResponse.reason);
            }

            const config = guardrailsConfig.value?.config || {};
            const scenarios = Array.isArray(guardrailsConfig.value?.supported_scenarios)
                ? guardrailsConfig.value.supported_scenarios.filter((value: string) => value && value !== '_global')
                : [];
            setSupportedScenarios(scenarios);
            setGroups(Array.isArray(config.groups) ? config.groups : []);
            setPolicies(Array.isArray(config.policies) ? config.policies : []);
            setBuiltins(Array.isArray(builtinResponse.value?.policies) ? builtinResponse.value.policies : []);
            setLoadError(null);
        } catch (error) {
            console.error('Failed to load guardrails config:', error);
            setGroups([]);
            setPolicies([]);
            setBuiltins([]);
            setSupportedScenarios([]);
            setLoadError('Failed to load guardrails config');
        } finally {
            if (!silent) {
                setLoading(false);
            }
        }
    };

    const loadRegistry = async (force = false) => {
        try {
            setRegistryLoading(true);
            setRegistryLoadError(null);
            const response = await api.getGuardrailsRegistry(force);
            if (response?.success === false) {
                throw new Error(response?.error || 'Failed to load registry');
            }
            setRegistryURL(response?.url || '');
            setRegistryPolicies(Array.isArray(response?.policies) ? response.policies : []);
            setRegistryLoadError(null);
        } catch (error: any) {
            setRegistryURL('');
            setRegistryPolicies([]);
            setRegistryLoadError(error?.message || 'Failed to load registry');
        } finally {
            setRegistryLoading(false);
        }
    };

    useEffect(() => {
        loadPolicies();
    }, []);

    useEffect(() => {
        loadRegistry();
    }, []);

    useEffect(() => {
        writePendingRegistryInstallIds(pendingRegistryInstallIds);
    }, [pendingRegistryInstallIds]);

    useEffect(() => {
        if (loading || loadError || initializingDefaultGroup || supportedScenarios.length === 0) {
            return;
        }
        if (groups.some((group) => group.id === DEFAULT_GROUP_ID)) {
            return;
        }

        const ensureDefaultGroup = async () => {
            try {
                setInitializingDefaultGroup(true);
                const result = await api.createGuardrailsGroup({
                    id: DEFAULT_GROUP_ID,
                    name: 'Default',
                    enabled: true,
                    severity: 'high',
                });
                if (!result?.success) {
                    setActionMessage({ type: 'error', text: result?.error || 'Failed to create default group' });
                    return;
                }
                await loadPolicies(true);
            } catch (error: any) {
                setActionMessage({ type: 'error', text: error?.message || 'Failed to create default group' });
            } finally {
                setInitializingDefaultGroup(false);
            }
        };

        ensureDefaultGroup();
    }, [groups, initializingDefaultGroup, loadError, loading, supportedScenarios]);

    useEffect(() => {
        const params = new URLSearchParams(location.search);
        const policyId = params.get('policyId') || params.get('ruleId');
        if (!policyId || policies.length === 0) {
            return;
        }
        const policy = policies.find((item) => item.id === policyId);
        if (!policy) {
            return;
        }
        const prepared = makeEditorState(policy);
        const nextState = prepared.state;
        setSelectedPolicyId(policy.id);
        setIsNewPolicy(false);
        setEditorOpen(true);
        setAdvancedOpen(false);
        setOversizedListFields(prepared.oversized);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(nextState));
        navigate('/guardrails/rules', { replace: true });
    }, [location.search, navigate, policies, scenarioOptions]);

    useEffect(() => {
        const draft = (location.state as { newPolicyDraft?: Partial<EditorState> } | null)?.newPolicyDraft;
        if (!draft) {
            return;
        }
        if (supportedScenarios.length === 0) {
            return;
        }
        const nextState = makeEditorStateFromDraft(draft);
        setSelectedPolicyId(null);
        setIsNewPolicy(true);
        setEditorOpen(true);
        setAdvancedOpen(false);
        setOversizedListFields({});
        setSelectedResourceRow(splitLines(nextState.resources).length > 0 ? 0 : -1);
        setSelectedCommandTermRow(splitLines(nextState.commandTerms).length > 0 ? 0 : -1);
        setSelectedPatternRow(splitLines(nextState.patterns).length > 0 ? 0 : -1);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(nextState));
        navigate('/guardrails/rules', { replace: true, state: null });
    }, [location.state, navigate, supportedScenarios]);

    const openPolicyEditor = (policy: DisplayPolicy) => {
        const builtin = policy.isBuiltin ? builtinMap.get(policy.id) : undefined;
        const isVirtualBuiltin = !!builtin && !policies.some((existing) => existing.id === policy.id);
        const prepared = isVirtualBuiltin ? makeEditorState(buildBuiltinPayload(builtin, false)) : makeEditorState(policy);
        const nextState = prepared.state;
        setSelectedPolicyId(isVirtualBuiltin ? null : policy.id);
        setIsNewPolicy(isVirtualBuiltin);
        setEditorOpen(true);
        setAdvancedOpen(false);
        setOversizedListFields(prepared.oversized);
        setSelectedResourceRow(splitLines(nextState.resources).length > 0 ? 0 : -1);
        setSelectedCommandTermRow(splitLines(nextState.commandTerms).length > 0 ? 0 : -1);
        setSelectedPatternRow(splitLines(nextState.patterns).length > 0 ? 0 : -1);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(nextState));
    };

    const handleNewPolicy = (kind?: 'resource_access' | 'command_execution' | 'content') => {
        const baseState = makeEditorState().state;
        const nextState = kind ? applyKindDefaults(kind, baseState) : baseState;
        setSelectedPolicyId(null);
        setIsNewPolicy(true);
        setEditorOpen(true);
        setAdvancedOpen(false);
        setOversizedListFields({});
        setSelectedResourceRow(splitLines(nextState.resources).length > 0 ? 0 : -1);
        setSelectedCommandTermRow(splitLines(nextState.commandTerms).length > 0 ? 0 : -1);
        setSelectedPatternRow(splitLines(nextState.patterns).length > 0 ? 0 : -1);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(nextState));
    };

    const handleSelectPolicyGroup = (groupID: string) => {
        setEditorState((state) => ({
            ...state,
            groups: state.groups.includes(groupID) ? state.groups.filter((value) => value !== groupID) : [...state.groups, groupID],
        }));
    };

    const buildPolicyPayload = (state: EditorState) => {
        const commandActions = state.actions.includes('install') ? ['install'] : ['execute'];
        const operationMatch = {
            tool_names: effectiveListValues('toolNames', state.toolNames),
            actions: {
                include:
                    state.kind === 'command_execution'
                        ? commandActions
                        : state.actions.filter((action) => action !== 'execute'),
            },
            terms: state.kind === 'command_execution' ? effectiveListValues('commandTerms', state.commandTerms) : [],
            resources: {
                type: 'path',
                mode: state.resourceMode,
                values: effectiveListValues('resources', state.resources),
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
                          patterns: effectiveListValues('patterns', state.patterns),
                          pattern_mode: state.patternMode,
                          case_sensitive: state.caseSensitive,
                      }
                    : operationMatch,
        };
        return payload;
    };

    const handleSavePolicy = async (): Promise<boolean> => {
        if (!editorState.kind) {
            setActionMessage({ type: 'error', text: 'Choose a policy kind first.' });
            return false;
        }
        if (!editorState.name.trim()) {
            setActionMessage({ type: 'error', text: 'Policy name is required before saving.' });
            return false;
        }
        const effectiveEditorState =
            editorState.id.trim() || !editorState.kind
                ? editorState
                : {
                      ...editorState,
                      id: generatePolicyId(editorState.name, editorState.kind, isNewPolicy ? undefined : selectedPolicyId || editorState.id),
                  };
        if (editorState.kind === 'content' && effectiveListValues('patterns', editorState.patterns).length === 0) {
            setActionMessage({ type: 'error', text: 'Privacy policies require at least one pattern.' });
            return false;
        }
        if (
            editorState.kind === 'resource_access' &&
            effectiveListValues('resources', editorState.resources).length === 0 &&
            editorState.actions.length === 0 &&
            effectiveListValues('toolNames', editorState.toolNames).length === 0
        ) {
            setActionMessage({ type: 'error', text: 'Resource access policies require at least one action, resource, or tool filter.' });
            return false;
        }
        if (
            editorState.kind === 'command_execution' &&
            effectiveListValues('commandTerms', editorState.commandTerms).length === 0 &&
            effectiveListValues('toolNames', editorState.toolNames).length === 0 &&
            effectiveListValues('resources', editorState.resources).length === 0
        ) {
            setActionMessage({ type: 'error', text: 'Command execution policies require a term match, tool filter, or resource filter.' });
            return false;
        }

        try {
            setPendingSave(true);
            const payload = buildPolicyPayload(effectiveEditorState);
            const targetPolicyId = isNewPolicy ? effectiveEditorState.id : (selectedPolicyId || effectiveEditorState.id);
            const result = isNewPolicy
                ? await api.createGuardrailsPolicy(payload)
                : await api.updateGuardrailsPolicy(targetPolicyId, payload);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to save policy' });
                return false;
            }
            await loadPolicies(true);
            setEditorState(effectiveEditorState);
            setSelectedPolicyId(effectiveEditorState.id);
            setIsNewPolicy(false);
            setEditorSnapshot(JSON.stringify(effectiveEditorState));
            setActionMessage({ type: 'success', text: `Policy "${effectiveEditorState.id}" saved.` });
            setEditorOpen(false);
            setConfirmCloseOpen(false);
            return true;
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to save policy' });
            return false;
        } finally {
            setPendingSave(false);
        }
    };

    const handleDuplicatePolicy = async () => {
        const existingIds = new Set(policies.map((policy) => policy.id));
        const baseId = `${editorState.id}-copy`;
        let nextId = baseId;
        let suffix = 2;
        while (existingIds.has(nextId)) {
            nextId = `${baseId}-${suffix}`;
            suffix += 1;
        }

        const nextState = {
            ...editorState,
            id: nextId,
            name: `${editorState.name} (copy)`,
        };

        // Duplicating now only creates a local draft. The copied policy is not
        // persisted until the user explicitly saves it.
        setSelectedPolicyId(null);
        setIsNewPolicy(true);
        setEditorOpen(true);
        setSelectedResourceRow(splitLines(nextState.resources).length > 0 ? 0 : -1);
        setSelectedCommandTermRow(splitLines(nextState.commandTerms).length > 0 ? 0 : -1);
        setSelectedPatternRow(splitLines(nextState.patterns).length > 0 ? 0 : -1);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(editorState));
        setActionMessage({ type: 'success', text: `Draft copy "${nextId}" is ready. Save to create it.` });
    };

    const handleTogglePolicy = async (policyId: string, enabled: boolean) => {
        try {
            setPendingPolicyId(policyId);
            const builtin = builtinMap.get(policyId);
            const installedPolicy = policies.find((policy) => policy.id === policyId);
            const result =
                !installedPolicy && builtin
                    ? await api.createGuardrailsPolicy(buildBuiltinPayload(builtin, enabled))
                    : await api.updateGuardrailsPolicy(policyId, {
                          enabled,
                          ...(enabled
                              ? { groups: ensureDefaultGroupMembership(installedPolicy?.groups) }
                              : {}),
                      });
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update policy' });
                return;
            }
            await loadPolicies(true);
            if (selectedPolicyId === policyId) {
                setEditorState((state) => ({
                    ...state,
                    enabled,
                    groups: enabled ? ensureDefaultGroupMembership(state.groups) : state.groups,
                }));
                setEditorSnapshot((snapshot) => {
                    if (!snapshot) {
                        return snapshot;
                    }
                    const nextSnapshot = JSON.parse(snapshot);
                    nextSnapshot.enabled = enabled;
                    if (enabled) {
                        nextSnapshot.groups = ensureDefaultGroupMembership(nextSnapshot.groups);
                    }
                    return JSON.stringify(nextSnapshot);
                });
            }
            setActionMessage({ type: 'success', text: `Policy "${policyId}" updated.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to update policy' });
        } finally {
            setPendingPolicyId(null);
        }
    };

    const handleSetPoliciesEnabled = async (enabled: boolean) => {
        try {
            setPendingBulkPolicyAction(enabled ? 'enable' : 'disable');
            const policiesToUpdate = selectedTabPolicies.filter((policy) =>
                enabled ? policyNeedsEnableWithDefault(policy) : policyNeedsDisable(policy)
            );
            if (policiesToUpdate.length === 0) {
                setActionMessage({
                    type: 'success',
                    text: enabled
                        ? `All ${selectedTabLabel} policies are already enabled and assigned to Default.`
                        : `All ${selectedTabLabel} policies are already disabled.`,
                });
                return;
            }
            const results: Array<{ id: string; result: any }> = [];
            for (const policy of policiesToUpdate) {
                const builtin = builtinMap.get(policy.id);
                const installedPolicy = policies.find((item) => item.id === policy.id);
                if (enabled && !installedPolicy && builtin) {
                    results.push({
                        id: policy.id,
                        result: await api.createGuardrailsPolicy(buildBuiltinPayload(builtin, true)),
                    });
                    continue;
                }
                results.push({
                    id: policy.id,
                    result: await api.updateGuardrailsPolicy(policy.id, {
                        enabled,
                        ...(enabled
                            ? { groups: ensureDefaultGroupMembership(installedPolicy?.groups) }
                            : {}),
                    }),
                });
            }

            const failed = results.filter(({ result }) => !result?.success);
            await loadPolicies(true);

            if (selectedPolicyId) {
                const selected = displayPolicies.find((policy) => policy.id === selectedPolicyId);
                if (selected) {
                    setEditorState((state) => ({
                        ...state,
                        enabled,
                        groups: enabled ? ensureDefaultGroupMembership(state.groups) : state.groups,
                    }));
                    setEditorSnapshot((snapshot) => {
                        if (!snapshot) {
                            return snapshot;
                        }
                        const nextSnapshot = JSON.parse(snapshot);
                        nextSnapshot.enabled = enabled;
                        if (enabled) {
                            nextSnapshot.groups = ensureDefaultGroupMembership(nextSnapshot.groups);
                        }
                        return JSON.stringify(nextSnapshot);
                    });
                }
            }

            if (failed.length > 0) {
                setActionMessage({
                    type: 'error',
                    text: `${enabled ? 'Enabled' : 'Disabled'} ${results.length - failed.length} ${selectedTabLabel} policies. ${failed.length} failed.`,
                });
                return;
            }

            setActionMessage({
                type: 'success',
                text: enabled
                    ? `Enabled ${results.length} ${selectedTabLabel} policies and assigned them to Default.`
                    : `Disabled ${results.length} ${selectedTabLabel} policies.`,
            });
        } catch (error: any) {
            setActionMessage({
                type: 'error',
                text: error?.message || `Failed to ${enabled ? 'enable' : 'disable'} ${selectedTabLabel.toLowerCase()} policies`,
            });
        } finally {
            setPendingBulkPolicyAction(null);
        }
    };

    const handleDeletePolicy = async () => {
        if (!deletePolicyId) {
            return;
        }
        try {
            setPendingPolicyId(deletePolicyId);
            const result = await api.deleteGuardrailsPolicy(deletePolicyId);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to delete policy' });
                return;
            }
            await loadPolicies(true);
            if (selectedPolicyId === deletePolicyId) {
                setSelectedPolicyId(null);
                setEditorOpen(false);
            }
            setActionMessage({ type: 'success', text: `Policy "${deletePolicyId}" deleted.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to delete policy' });
        } finally {
            setPendingPolicyId(null);
            setDeletePolicyId(null);
        }
    };

    const handleInstallRegistryPolicy = async (policyId: string) => {
        if (pendingRegistryInstallIds.has(policyId)) {
            return;
        }
        try {
            addPendingRegistryInstallId(policyId);
            setPendingRegistryInstallIds((prev) => {
                const next = new Set(prev);
                next.add(policyId);
                return next;
            });
            const result = await api.installGuardrailsRegistryPolicy(policyId);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to install policy' });
                return;
            }
            await loadPolicies(true);
            setActionMessage({ type: 'success', text: `Policy "${policyId}" installed.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to install policy' });
        } finally {
            removePendingRegistryInstallId(policyId);
            setPendingRegistryInstallIds((prev) => {
                if (!prev.has(policyId)) {
                    return prev;
                }
                const next = new Set(prev);
                next.delete(policyId);
                return next;
            });
        }
    };

    const handleCloseEditor = () => {
        if (isEditorDirty) {
            setConfirmCloseOpen(true);
            return;
        }
        setEditorOpen(false);
        blurActiveElement();
    };

    const handleConfirmClose = async (action: 'save' | 'discard' | 'cancel') => {
        if (action === 'cancel') {
            setConfirmCloseOpen(false);
            return;
        }
        if (action === 'save') {
            const saved = await handleSavePolicy();
            if (!saved) {
                return;
            }
        }
        setConfirmCloseOpen(false);
        setEditorOpen(false);
        blurActiveElement();
    };

    const renderPolicySection = (
        title: string,
        description: string,
        items: DisplayPolicy[],
        kind: 'resource_access' | 'command_execution' | 'content'
    ) => (
        <Box sx={{ mb: 3 }}>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                {description}
            </Typography>
            {items.length === 0 ? (
                <Box sx={{ border: '1px dashed', borderColor: 'divider', borderRadius: 2 }}>
                    <EmptyStateGuide
                        title={
                            kind === 'resource_access'
                                ? 'No resource access policies yet'
                                : kind === 'command_execution'
                                  ? 'No command execution policies yet'
                                  : 'No privacy policies yet'
                        }
                        description={
                            kind === 'resource_access'
                                ? 'Start with a guided resource access policy to control reads, writes, deletes, and protected paths.'
                                : kind === 'command_execution'
                                  ? 'Start with a guided command execution policy to control dangerous or disallowed commands.'
                                  : 'Start with a guided privacy policy to filter model output or tool results.'
                        }
                        showOAuthButton={false}
                        showHeroIcon={false}
                        primaryButtonLabel="New Policy"
                        onAddApiKeyClick={() => handleNewPolicy(kind)}
                    />
                </Box>
            ) : (
                <List dense sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, py: 0, overflow: 'hidden' }}>
                    {items.map((policy) => {
                        const effectiveState = getEffectivePolicyState(policy);
                        return (
                            <ListItem
                                key={policy.id}
                                sx={{
                                    px: 0,
                                    py: 0,
                                    borderBottom: '1px solid',
                                    borderColor: 'divider',
                                    '&:last-child': { borderBottom: 'none' },
                                }}
                            >
                                <Box
                                    sx={{
                                        display: 'flex',
                                        alignItems: 'flex-start',
                                        flexDirection: { xs: 'column', lg: 'row' },
                                        gap: 1.5,
                                        width: '100%',
                                        cursor: 'pointer',
                                        px: 2,
                                        py: 1.5,
                                        bgcolor: selectedPolicyId === policy.id ? 'action.selected' : 'transparent',
                                        '&:hover': { bgcolor: 'action.hover' },
                                        opacity: effectiveState.inheritedDisabled ? 0.65 : 1,
                                    }}
                                    onClick={() => openPolicyEditor(policy)}
                                >
                                    <Box sx={{ minWidth: { lg: 220 }, flexShrink: 0, minHeight: 0 }}>
                                        <Stack direction="row" spacing={0.75} alignItems="center" useFlexGap flexWrap="wrap">
                                            <Typography
                                                variant="body2"
                                                sx={{ fontWeight: 600, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: '100%' }}
                                            >
                                                {policy.id}
                                            </Typography>
                                            {policy.isBuiltin && (
                                                <Chip
                                                    size="small"
                                                    label="Built-in"
                                                    variant="outlined"
                                                    sx={{ height: 20, fontSize: '0.7rem', '& .MuiChip-label': { px: 0.5 } }}
                                                />
                                            )}
                                            {effectiveState.inheritedDisabled && (
                                                <Chip size="small" label="No active group" variant="outlined" />
                                            )}
                                        </Stack>
                                        <Typography
                                            variant="caption"
                                            color="text.secondary"
                                            sx={{ display: 'block', mt: 0.5, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                                        >
                                            {policy.name || 'Unnamed policy'}
                                        </Typography>
                                    </Box>

                                    <Box sx={{ flex: 1, minWidth: 0 }}>
                                        <Typography
                                            variant="body2"
                                            color="text.primary"
                                            sx={{
                                                whiteSpace: 'nowrap',
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                            }}
                                        >
                                            {buildPolicySummary(policy)}
                                        </Typography>
                                    </Box>

                                    <Stack
                                        direction={{ xs: 'row', sm: 'row' }}
                                        spacing={1}
                                        alignItems="center"
                                        sx={{
                                            width: { xs: '100%', lg: 220 },
                                            minWidth: { lg: 220 },
                                            justifyContent: { xs: 'space-between', lg: 'flex-end' },
                                            flexShrink: 0,
                                            alignSelf: 'center',
                                        }}
                                    >
                                        <Chip
                                            size="small"
                                            label={
                                                effectiveState.inheritedDisabled
                                                    ? 'No active group'
                                                    : policy.enabled !== true
                                                      ? 'Disabled'
                                                      : 'Enabled'
                                            }
                                        />
                                        <FormControlLabel
                                            sx={{ ml: 0 }}
                                            onClick={(e) => e.stopPropagation()}
                                            control={
                                                <Switch
                                                    size="small"
                                                    checked={policy.enabled === true}
                                                    disabled={pendingPolicyId === policy.id}
                                                    onChange={(e) => handleTogglePolicy(policy.id, e.target.checked)}
                                                />
                                            }
                                            label="Enabled"
                                        />
                                        <Box sx={{ width: 32, display: 'flex', justifyContent: 'center', flexShrink: 0 }}>
                                            {!policy.isBuiltin && (
                                                <Tooltip title="Delete policy" arrow>
                                                    <span>
                                                        <IconButton
                                                            size="small"
                                                            disabled={pendingPolicyId === policy.id}
                                                            onClick={(e) => {
                                                                e.stopPropagation();
                                                                setDeletePolicyId(policy.id);
                                                            }}
                                                        >
                                                            <DeleteOutline fontSize="small" />
                                                        </IconButton>
                                                    </span>
                                                </Tooltip>
                                            )}
                                        </Box>
                                    </Stack>
                                </Box>
                            </ListItem>
                        );
                    })}
                </List>
            )}
        </Box>
    );

    const renderCompactListEditor = ({
        title,
        description,
        columnLabel,
        value,
        selectedIndex,
        onSelectedIndexChange,
        onChange,
        placeholder,
        helperText,
        oversizedField,
    }: {
        title: string;
        description: string;
        columnLabel: string;
        value: string;
        selectedIndex: number;
        onSelectedIndexChange: (index: number) => void;
        onChange: (value: string) => void;
        placeholder: string;
        helperText: string;
        oversizedField?: OversizedListField;
    }) => {
        const rows = textListRows(value);
        const isEmpty = rows.length === 1 && rows[0] === '';
        const showEmptyState = isEmpty && selectedIndex < 0;
        const visibleRows = showEmptyState ? [] : rows;
        const editablePrefixCount = oversizedField ? Math.max(0, rows.length - oversizedField.preview.length) : rows.length;
        const canRemove = !showEmptyState && (!oversizedField || selectedIndex < editablePrefixCount);

        return (
            <Stack spacing={1.5}>
                <Box>
                    <Typography variant="subtitle2">{title}</Typography>
                    <Typography variant="caption" color="text.secondary">
                        {description}
                    </Typography>
                </Box>
                <TableContainer component={Paper} variant="outlined" sx={{ borderRadius: 2, boxShadow: 'none' }}>
                    <Stack
                        direction="row"
                        spacing={0.5}
                        justifyContent="space-between"
                        alignItems="center"
                        sx={{
                            px: 1,
                            py: 0.5,
                            borderBottom: '1px solid',
                            borderColor: 'divider',
                            bgcolor: 'action.hover',
                        }}
                    >
                        <Stack direction="row" spacing={0.5}>
                            <IconButton
                                size="small"
                                color="primary"
                                onClick={() => {
                                    if (showEmptyState) {
                                        onSelectedIndexChange(0);
                                        return;
                                    }
                                    if (oversizedField) {
                                        onChange(['', ...rows].join('\n'));
                                        onSelectedIndexChange(0);
                                        return;
                                    }
                                    onChange(appendTextListValue(value));
                                    onSelectedIndexChange(rows.length);
                                }}
                            >
                                <Add fontSize="small" />
                            </IconButton>
                            <IconButton
                                size="small"
                                disabled={!canRemove}
                                onClick={() => {
                                    if (showEmptyState) {
                                        return;
                                    }
                                    if (oversizedField && selectedIndex >= editablePrefixCount) {
                                        return;
                                    }
                                    const index = Math.min(selectedIndex, rows.length - 1);
                                    const nextValue = removeTextListValue(value, index);
                                    onChange(nextValue);
                                    const nextRows = textListRows(nextValue);
                                    if (nextRows.length === 1 && nextRows[0] === '') {
                                        onSelectedIndexChange(-1);
                                    } else {
                                        onSelectedIndexChange(Math.max(0, Math.min(selectedIndex - 1, nextRows.length - 1)));
                                    }
                                }}
                            >
                                <Remove fontSize="small" />
                            </IconButton>
                        </Stack>
                    </Stack>
                    <Table size="small">
                        <TableHead>
                            <TableRow>
                                <TableCell sx={{ fontWeight: 600 }}>{columnLabel}</TableCell>
                            </TableRow>
                        </TableHead>
                        <TableBody>
                            {showEmptyState ? (
                                <TableRow>
                                    <TableCell sx={{ py: 3, textAlign: 'center', color: 'text.secondary' }}>
                                        No entries
                                    </TableCell>
                                </TableRow>
                            ) : (
                                visibleRows.map((item, index) => (
                                    <TableRow
                                        key={`${title}-${index}`}
                                        hover
                                        selected={selectedIndex === index}
                                        onClick={() => onSelectedIndexChange(index)}
                                        sx={{ cursor: 'pointer' }}
                                    >
                                        <TableCell sx={{ py: 0.5 }}>
                                            <InputBase
                                                fullWidth
                                                value={item}
                                                readOnly={Boolean(oversizedField) && index >= editablePrefixCount}
                                                placeholder={index === 0 ? placeholder : 'Add another entry'}
                                                onFocus={() => onSelectedIndexChange(index)}
                                                onChange={(e) => onChange(updateTextListValue(value, index, e.target.value))}
                                                sx={{ fontSize: '0.9rem' }}
                                            />
                                        </TableCell>
                                    </TableRow>
                                ))
                            )}
                        </TableBody>
                    </Table>
                </TableContainer>
                <FormHelperText>
                    {oversizedField
                        ? `This field contains ${oversizedField.total.toLocaleString()} entries. Showing the first ${oversizedField.preview.length} existing entries. Existing rows stay read-only, but you can prepend new entries without loading the full list. Saving preserves the rest of the list.`
                        : helperText}
                </FormHelperText>
            </Stack>
        );
    };

    const getScenarioPresentation = (scenario: string) => {
        switch (scenario) {
            case 'anthropic':
                return {
                    label: 'Anthropic',
                    description: 'Anthropic-compatible requests and responses.',
                    icon: <Anthropic size={18} />,
                };
            case 'claude_code':
                return {
                    label: 'Claude Code',
                    description: 'Tool-enabled Claude Code sessions and command workflows.',
                    icon: <Claude size={18} />,
                };
            case 'openai':
                return {
                    label: 'OpenAI',
                    description: 'OpenAI-compatible requests and responses.',
                    icon: <OpenAI size={18} />,
                };
            case 'opencode':
                return {
                    label: 'OpenCode',
                    description: 'OpenCode scenario traffic and agent flows.',
                    icon: <CodeIcon sx={{ fontSize: 18 }} />,
                };
            case 'xcode':
                return {
                    label: 'Xcode',
                    description: 'Xcode-integrated coding workflows.',
                    icon: <LaptopMac sx={{ fontSize: 18 }} />,
                };
            case 'agent':
                return {
                    label: 'Agent',
                    description: 'Agent-style orchestration and assistant flows.',
                    icon: <AutoAwesome sx={{ fontSize: 18 }} />,
                };
            default: {
                const label = scenario
                    .split('_')
                    .filter(Boolean)
                    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
                    .join(' ');
                return {
                    label,
                    description: `${label} scenario traffic.`,
                    icon: <Rule sx={{ fontSize: 18 }} color="action" />,
                };
            }
        }
    };

    const renderScenarioScopeSelector = ({
        title,
        description,
        value,
        onChange,
        helperText,
    }: {
        title: string;
        description: string;
        value: string[];
        onChange: (value: string[]) => void;
        helperText: string;
    }) => (
        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
            <Stack spacing={1.5}>
                <Box>
                    <Typography variant="subtitle2">{title}</Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                        {description}
                    </Typography>
                </Box>
                <Box
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                        gap: 1.5,
                    }}
                >
                    {scenarioOptions.map((option) => {
                        const selected = value.includes(option);
                        const presentation = getScenarioPresentation(option);
                        return (
                            <Box
                                key={option}
                                onClick={() => onChange(toggleValue(value, option))}
                                sx={{
                                    border: '1px solid',
                                    borderColor: selected ? 'primary.main' : 'divider',
                                    bgcolor: selected ? 'action.selected' : 'background.paper',
                                    borderRadius: 2,
                                    p: 1.5,
                                    cursor: 'pointer',
                                    transition: 'all 0.15s ease',
                                    '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                }}
                            >
                                <Stack spacing={0.75}>
                                    <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                        {presentation.icon}
                                        <Typography variant="body2" fontWeight={600}>
                                            {presentation.label}
                                        </Typography>
                                        {selected && (
                                            <Tooltip title="Selected">
                                                <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                            </Tooltip>
                                        )}
                                    </Stack>
                                    <Typography variant="caption" color="text.secondary">
                                        {presentation.description}
                                    </Typography>
                                </Stack>
                            </Box>
                        );
                    })}
                </Box>
                <FormHelperText>{helperText}</FormHelperText>
            </Stack>
        </Box>
    );

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Policies"
                    subtitle="Policies define concrete rules. Groups are managed separately and control which policy sets are active. Built-in policies are marked directly in the list."
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button variant="outlined" size="small" onClick={() => navigate('/guardrails/groups')}>
                                Manage Groups
                            </Button>
                        </Stack>
                    }
                >
                    <Stack spacing={1.5}>
                        {loadError && <Alert severity="error">{loadError}</Alert>}
                        {actionMessage && !editorOpen && (
                            <Alert severity={actionMessage.type}>{actionMessage.text}</Alert>
                        )}
                    </Stack>
                </UnifiedCard>

                <UnifiedCard
                    title="Policies"
                    subtitle={`${policies.length} polic${policies.length === 1 ? 'y' : 'ies'} configured`}
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button
                                variant="outlined"
                                size="small"
                                onClick={() => handleSetPoliciesEnabled(true)}
                                disabled={pendingBulkPolicyAction !== null || selectedTabPolicies.length === 0}
                            >
                                Enable All
                            </Button>
                            <Button
                                variant="outlined"
                                size="small"
                                onClick={() => handleSetPoliciesEnabled(false)}
                                disabled={pendingBulkPolicyAction !== null || selectedTabPolicies.length === 0}
                            >
                                Disable All
                            </Button>
                            <Button
                                variant="contained"
                                size="small"
                                startIcon={<Rule />}
                                onClick={() => handleNewPolicy(selectedPolicyTab)}
                            >
                                New Policy
                            </Button>
                        </Stack>
                    }
                >
                    <Stack spacing={2}>
                        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
                            <Tabs
                                value={selectedPolicyTab}
                                onChange={(_, value) => setSelectedPolicyTab(value)}
                                variant="scrollable"
                                scrollButtons="auto"
                            >
                                <Tab value="resource_access" label={`Resource Access (${resourceAccessPolicies.length})`} />
                                <Tab value="command_execution" label={`Command Execution (${commandExecutionPolicies.length})`} />
                                <Tab value="content" label={`Privacy (${contentPolicies.length})`} />
                            </Tabs>
                        </Box>
                        {selectedPolicyTab === 'resource_access' &&
                            renderPolicySection(
                                'Resource Access Policies',
                                'Use these to control reads, writes, deletes, and other path or resource access behaviors.',
                                resourceAccessPolicies,
                                'resource_access'
                            )}
                        {selectedPolicyTab === 'command_execution' &&
                            renderPolicySection(
                                'Command Execution Policies',
                                'Use these to control dangerous command execution patterns and shell behavior.',
                                commandExecutionPolicies,
                                'command_execution'
                            )}
                        {selectedPolicyTab === 'content' &&
                            renderPolicySection(
                                'Privacy Policies',
                                'Use these to filter model output and tool results before they are shown or forwarded.',
                                contentPolicies,
                                'content'
                            )}
                    </Stack>
                </UnifiedCard>

                <UnifiedCard
                    title="Download Management"
                    subtitle="Install additional policy fragments from the curated remote registry. After install, enable or disable them from the policy list above."
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button
                                variant="outlined"
                                size="small"
                                startIcon={<Refresh />}
                                disabled={registryLoading}
                                onClick={() => loadRegistry(true)}
                            >
                                {registryLoading ? 'Refreshing…' : 'Retry'}
                            </Button>
                            {registryURL ? (
                                <Button
                                    variant="outlined"
                                    size="small"
                                    component="a"
                                    href={registryURL}
                                    target="_blank"
                                    rel="noreferrer"
                                    startIcon={<ArticleOutlined />}
                                >
                                    Open Registry
                                </Button>
                            ) : null}
                        </Stack>
                    }
                >
                    <Stack spacing={2}>
                        {registryLoadError && <Alert severity="warning">{registryLoadError}</Alert>}
                        {!registryLoadError && registryLoading && (
                            <Alert severity="info">Loading remote registry…</Alert>
                        )}
                        {!registryLoadError && !registryLoading && downloadablePolicies.length === 0 && (
                            <Alert severity="info">No downloadable policies are currently listed in the remote registry.</Alert>
                        )}
                        {!registryLoadError && !registryLoading && downloadablePolicies.length > 0 && (
                            <List dense sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, py: 0, overflow: 'hidden' }}>
                                {downloadablePolicies.map((policy) => {
                                    const installedPolicy = policies.find((item) => item.id === policy.id);
                                    const isInstalled = Boolean(installedPolicy);
                                    const isEnabled = installedPolicy?.enabled === true;
                                    const isInstalling = pendingRegistryInstallIds.has(policy.id);
                                    return (
                                    <ListItem
                                        key={policy.id}
                                        sx={{
                                            px: 2,
                                            py: 1.5,
                                            borderBottom: '1px solid',
                                            borderColor: 'divider',
                                            '&:last-child': { borderBottom: 'none' },
                                        }}
                                    >
                                        <Box sx={{ display: 'flex', alignItems: 'flex-start', flexDirection: { xs: 'column', md: 'row' }, gap: 1.5, width: '100%' }}>
                                            <Box sx={{ minWidth: { md: 240 }, flexShrink: 0 }}>
                                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                    {policy.id}
                                                </Typography>
                                                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                                                    {policy.name || policy.path}
                                                </Typography>
                                            </Box>
                                            <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'center' }}>
                                                <Typography variant="body2" color="text.primary">
                                                    {policy.reason || 'Download this policy fragment from the remote registry and manage enable/disable from the local policy list.'}
                                                </Typography>
                                            </Box>
                                            <Box sx={{ width: { xs: '100%', md: 'auto' }, display: 'flex', justifyContent: { xs: 'flex-start', md: 'flex-end' }, alignItems: 'center', alignSelf: 'center' }}>
                                                {isInstalled && isEnabled && (
                                                    <Chip size="small" color="success" label="Enabled" sx={{ mr: 1 }} />
                                                )}
                                                <Button
                                                    variant={isInstalled ? 'outlined' : 'contained'}
                                                    size="small"
                                                    disabled={isInstalled || isInstalling}
                                                    onClick={() => handleInstallRegistryPolicy(policy.id)}
                                                >
                                                    {isInstalled ? 'Installed' : isInstalling ? 'Installing…' : 'Install'}
                                                </Button>
                                            </Box>
                                        </Box>
                                    </ListItem>
                                    );
                                })}
                            </List>
                        )}
                    </Stack>
                </UnifiedCard>
            </Stack>

            <Dialog open={editorOpen} onClose={handleCloseEditor} disableRestoreFocus fullWidth maxWidth="md">
                <DialogTitle>{isNewPolicy ? 'New Policy' : `Edit Policy${selectedPolicyId ? ` · ${selectedPolicyId}` : ''}`}</DialogTitle>
                <DialogContent dividers>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        {actionMessage && <Alert severity={actionMessage.type}>{actionMessage.text}</Alert>}
                        <Stack spacing={1}>
                            <Stack direction="row" spacing={0.75} alignItems="center">
                                <Typography variant="subtitle2">Basic Settings</Typography>
                                <Tooltip title="Choose the policy type first, then fill in only the fields that apply to that type.">
                                    <IconButton size="small" sx={{ p: 0.25 }}>
                                        <HelpOutline fontSize="inherit" />
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                            <Box
                                sx={{
                                    display: 'grid',
                                    gridTemplateColumns: { xs: '1fr', md: '1fr', lg: '1fr 1fr 1fr' },
                                    gap: 2,
                                }}
                            >
                                <Box
                                    onClick={() => setEditorState((state) => applyKindDefaults('resource_access', state))}
                                    sx={{
                                        border: '1px solid',
                                        borderColor: editorState.kind === 'resource_access' ? 'primary.main' : 'divider',
                                        bgcolor: editorState.kind === 'resource_access' ? 'action.selected' : 'background.paper',
                                        borderRadius: 2,
                                        p: 2,
                                        cursor: 'pointer',
                                        transition: 'all 0.15s ease',
                                        '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                    }}
                                >
                                    <Stack spacing={1}>
                                        <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                            <Terminal fontSize="small" color={editorState.kind === 'resource_access' ? 'primary' : 'action'} />
                                            <Typography variant="subtitle2">Resource Access</Typography>
                                            <Tooltip title="Inspect access to files, directories, and other resources. Best for reads, writes, deletes, and protected paths like ~/.ssh or .env.">
                                                <IconButton size="small" sx={{ p: 0.25 }}>
                                                    <HelpOutline fontSize="inherit" />
                                                </IconButton>
                                            </Tooltip>
                                            {editorState.kind === 'resource_access' && (
                                                <Tooltip title="Selected">
                                                    <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                </Tooltip>
                                            )}
                                        </Stack>
                                    </Stack>
                                </Box>

                                <Box
                                    onClick={() => setEditorState((state) => applyKindDefaults('command_execution', state))}
                                    sx={{
                                        border: '1px solid',
                                        borderColor: editorState.kind === 'command_execution' ? 'primary.main' : 'divider',
                                        bgcolor: editorState.kind === 'command_execution' ? 'action.selected' : 'background.paper',
                                        borderRadius: 2,
                                        p: 2,
                                        cursor: 'pointer',
                                        transition: 'all 0.15s ease',
                                        '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                    }}
                                >
                                    <Stack spacing={1}>
                                        <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                            <Terminal fontSize="small" color={editorState.kind === 'command_execution' ? 'primary' : 'action'} />
                                            <Typography variant="subtitle2" sx={{ whiteSpace: 'nowrap' }}>
                                                Command Execution
                                            </Typography>
                                            <Tooltip title="Inspect commands the model wants to run. Best for dangerous shell commands, execution patterns, and risky programs like rm -rf or curl | sh.">
                                                <IconButton size="small" sx={{ p: 0.25 }}>
                                                    <HelpOutline fontSize="inherit" />
                                                </IconButton>
                                            </Tooltip>
                                            {editorState.kind === 'command_execution' && (
                                                <Tooltip title="Selected">
                                                    <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                </Tooltip>
                                            )}
                                        </Stack>
                                    </Stack>
                                </Box>

                                <Box
                                    onClick={() => setEditorState((state) => applyKindDefaults('content', state))}
                                    sx={{
                                        border: '1px solid',
                                        borderColor: editorState.kind === 'content' ? 'primary.main' : 'divider',
                                        bgcolor: editorState.kind === 'content' ? 'action.selected' : 'background.paper',
                                        borderRadius: 2,
                                        p: 2,
                                        cursor: 'pointer',
                                        transition: 'all 0.15s ease',
                                        '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                    }}
                                >
                                    <Stack spacing={1}>
                                        <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                            <ArticleOutlined fontSize="small" color={editorState.kind === 'content' ? 'primary' : 'action'} />
                                            <Typography variant="subtitle2">Privacy Policy</Typography>
                                            <Tooltip title="Inspect returned text from the model or tools. Use privacy patterns to review or block sensitive content.">
                                                <IconButton size="small" sx={{ p: 0.25 }}>
                                                    <HelpOutline fontSize="inherit" />
                                                </IconButton>
                                            </Tooltip>
                                            {editorState.kind === 'content' && (
                                                <Tooltip title="Selected">
                                                    <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                </Tooltip>
                                            )}
                                        </Stack>
                                    </Stack>
                                </Box>
                            </Box>
                        </Stack>

                        <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                                <TextField
                                    label="Name"
                                    size="small"
                                    fullWidth
                                    value={editorState.name}
                                onChange={(e) =>
                                    setEditorState((state) => {
                                        const name = e.target.value;
                                        return {
                                            ...state,
                                            name,
                                            id: isNewPolicy ? generatePolicyId(name, state.kind) : state.id,
                                        };
                                    })
                                    }
                                    helperText="Required. Choose a clear name before saving."
                                    placeholder={
                                        editorState.kind === 'resource_access'
                                            ? 'Example: Block SSH directory reads'
                                            : editorState.kind === 'command_execution'
                                              ? 'Example: Block destructive rm commands'
                                              : editorState.kind === 'content'
                                                ? 'Example: Block private key output'
                                                : 'Enter a policy name'
                                    }
                                    disabled={!editorState.kind}
                                />
                        </Stack>

                        {editorState.kind ? (
                            <>
                                <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                                    <Stack spacing={1.25}>
                                        <Box>
                                            <Typography variant="subtitle2">Assign Groups</Typography>
                                            <Box
                                                sx={{
                                                    display: 'grid',
                                                    gridTemplateColumns: { xs: '1fr', md: '1fr 1fr', lg: '1fr 1fr 1fr' },
                                                    gap: 1,
                                                    mt: 1,
                                                }}
                                            >
                                                {groupOptions.map((option) => {
                                                    const selected = editorState.groups.includes(option.value);
                                                    return (
                                                        <Box
                                                            key={option.value}
                                                            onClick={() => handleSelectPolicyGroup(option.value)}
                                                            sx={{
                                                                border: '1px solid',
                                                                borderColor: selected ? 'primary.main' : 'divider',
                                                                bgcolor: selected ? 'action.selected' : 'background.paper',
                                                                borderRadius: 2,
                                                                p: 1.25,
                                                                cursor: 'pointer',
                                                                transition: 'all 0.15s ease',
                                                                '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                                            }}
                                                        >
                                                            <Stack direction="row" spacing={0.75} alignItems="center" useFlexGap flexWrap="wrap">
                                                                <Typography variant="subtitle2">
                                                                    {option.label}
                                                                </Typography>
                                                                {selected && (
                                                                    <Tooltip title="Selected">
                                                                        <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                                    </Tooltip>
                                                                )}
                                                            </Stack>
                                                        </Box>
                                                    );
                                                })}
                                            </Box>
                                        </Box>
                                    </Stack>
                                </Box>

                                {editorState.kind === 'resource_access' ? (
                                    <Box
                                        sx={{
                                            display: 'grid',
                                            gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                                            gap: 2,
                                        }}
                                    >
                                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, gridColumn: { md: '1 / span 2' } }}>
                                            <Stack spacing={1.5}>
                                                <Stack direction="row" spacing={0.75} alignItems="center">
                                                    <Typography variant="subtitle2">Choose Actions</Typography>
                                                    <Tooltip title="Choose the type of resource access you want to control. These actions focus on files, directories, and other protected resources.">
                                                        <IconButton size="small" sx={{ p: 0.25 }}>
                                                            <HelpOutline fontSize="inherit" />
                                                        </IconButton>
                                                    </Tooltip>
                                                </Stack>
                                                <Box
                                                    sx={{
                                                        display: 'grid',
                                                        gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                                                        gap: 1.5,
                                                    }}
                                                >
                                                    {resourceAccessActionOptions.map((option) => {
                                                        const selected = editorState.actions.includes(option.value);
                                                        return (
                                                            <Box
                                                                key={option.value}
                                                                onClick={() =>
                                                                    setEditorState((state) => ({
                                                                        ...state,
                                                                        actions: toggleValue(state.actions, option.value),
                                                                    }))
                                                                }
                                                                sx={{
                                                                    border: '1px solid',
                                                                    borderColor: selected ? 'primary.main' : 'divider',
                                                                    bgcolor: selected ? 'action.selected' : 'background.paper',
                                                                    borderRadius: 2,
                                                                    p: 1.5,
                                                                    cursor: 'pointer',
                                                                    transition: 'all 0.15s ease',
                                                                    '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                                                }}
                                                            >
                                                                <Stack spacing={0.75}>
                                                                    <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                                                        <Typography variant="body2" fontWeight={600}>
                                                                            {option.label}
                                                                        </Typography>
                                                                        {selected && (
                                                                            <Tooltip title="Selected">
                                                                                <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                                            </Tooltip>
                                                                        )}
                                                                    </Stack>
                                                                    <Typography variant="caption" color="text.secondary">
                                                                        {option.description}
                                                                    </Typography>
                                                                </Stack>
                                                            </Box>
                                                        );
                                                    })}
                                                </Box>
                                                <FormHelperText>
                                                    `Command Execution` policies use `execute` or `install`, so those categories are not shown here. Shell redirection is treated as `write`.
                                                </FormHelperText>
                                            </Stack>
                                        </Box>

                                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, gridColumn: { md: '1 / span 2' } }}>
                                            <Stack spacing={1.5}>
                                                {renderCompactListEditor({
                                                    title: 'Protected Resources',
                                                    description: 'Define the files, directories, URLs, or other resources this policy protects.',
                                                    columnLabel: 'Path / URL / Resource',
                                                    value: editorState.resources,
                                                    oversizedField: oversizedListFields.resources,
                                                    selectedIndex: selectedResourceRow,
                                                    onSelectedIndexChange: setSelectedResourceRow,
                                                    onChange: (resources) => setEditorState((state) => ({ ...state, resources })),
                                                    placeholder: '~/.ssh',
                                                    helperText: 'Add one resource per row, such as `~/.ssh`, `.env`, `/etc/ssh`, or `https://api.example.com`.',
                                                })}
                                                <FormControl size="small" fullWidth>
                                                    <InputLabel id="resource-mode">Resource Match</InputLabel>
                                                    <Select
                                                        labelId="resource-mode"
                                                        label="Resource Match"
                                                        value={editorState.resourceMode}
                                                        onChange={(e) => setEditorState((state) => ({ ...state, resourceMode: String(e.target.value) }))}
                                                    >
                                                        <MenuItem value="prefix">prefix</MenuItem>
                                                        <MenuItem value="contains">contains</MenuItem>
                                                        <MenuItem value="exact">exact</MenuItem>
                                                    </Select>
                                                    <FormHelperText>
                                                        This match mode currently applies to every resource in the list. `prefix` is usually the safest default for path-oriented resources.
                                                    </FormHelperText>
                                                </FormControl>
                                            </Stack>
                                        </Box>
                                    </Box>
                                ) : editorState.kind === 'command_execution' ? (
                                    <Box
                                        sx={{
                                            display: 'grid',
                                            gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                                            gap: 2,
                                        }}
                                    >
                                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, gridColumn: { md: '1 / span 2' } }}>
                                            <Stack spacing={1.5}>
                                                <Stack direction="row" spacing={0.75} alignItems="center">
                                                    <Typography variant="subtitle2">Command Category</Typography>
                                                    <Tooltip title="Choose whether this policy targets general command execution patterns or normalized install commands.">
                                                        <IconButton size="small" sx={{ p: 0.25 }}>
                                                            <HelpOutline fontSize="inherit" />
                                                        </IconButton>
                                                    </Tooltip>
                                                </Stack>
                                                <Box
                                                    sx={{
                                                        display: 'grid',
                                                        gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                                                        gap: 1.5,
                                                    }}
                                                >
                                                    {commandExecutionActionOptions.map((option) => {
                                                        const selected = editorState.actions.includes(option.value);
                                                        return (
                                                            <Box
                                                                key={option.value}
                                                                onClick={() =>
                                                                    setEditorState((state) => ({
                                                                        ...state,
                                                                        actions: [option.value],
                                                                    }))
                                                                }
                                                                sx={{
                                                                    border: '1px solid',
                                                                    borderColor: selected ? 'primary.main' : 'divider',
                                                                    bgcolor: selected ? 'action.selected' : 'background.paper',
                                                                    borderRadius: 2,
                                                                    p: 1.5,
                                                                    cursor: 'pointer',
                                                                    transition: 'all 0.15s ease',
                                                                    '&:hover': { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                                                }}
                                                            >
                                                                <Stack spacing={0.75}>
                                                                    <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                                                        <Typography variant="body2" fontWeight={600}>
                                                                            {option.label}
                                                                        </Typography>
                                                                        {selected && (
                                                                            <Tooltip title="Selected">
                                                                                <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                                            </Tooltip>
                                                                        )}
                                                                    </Stack>
                                                                    <Typography variant="caption" color="text.secondary">
                                                                        {option.description}
                                                                    </Typography>
                                                                </Stack>
                                                            </Box>
                                                        );
                                                    })}
                                                </Box>
                                            </Stack>
                                        </Box>

                                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, gridColumn: { md: '1 / span 2' } }}>
                                            {renderCompactListEditor({
                                                title: editorState.actions.includes('install') ? 'Install Match' : 'Command Match',
                                                description:
                                                    editorState.actions.includes('install')
                                                        ? 'Describe the install targets you want to block or review. This matches normalized command terms such as package, crate, gem, or extension names.'
                                                        : 'Describe the command patterns you want to block or review. This is the main selector for execute policies.',
                                                columnLabel: editorState.actions.includes('install') ? 'Install Term' : 'Command Pattern',
                                                value: editorState.commandTerms,
                                                oversizedField: oversizedListFields.commandTerms,
                                                selectedIndex: selectedCommandTermRow,
                                                onSelectedIndexChange: setSelectedCommandTermRow,
                                                onChange: (commandTerms) => setEditorState((state) => ({ ...state, commandTerms })),
                                                placeholder: editorState.actions.includes('install') ? 'left-pad' : 'rm -rf',
                                                helperText: editorState.actions.includes('install')
                                                    ? 'One term per row, such as `left-pad`, `requests`, `ripgrep`, or `ms-python.python`.'
                                                    : 'One pattern per row, such as `rm -rf`, `curl | sh`, or `python -c`.',
                                            })}
                                        </Box>

                                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, gridColumn: { md: '1 / span 2' } }}>
                                            <Stack spacing={1.5}>
                                                {renderCompactListEditor({
                                                    title: 'Limit To Resources',
                                                    description:
                                                        editorState.actions.includes('install')
                                                            ? 'Optional. Add paths or resources only when the install rule should be limited to specific targets.'
                                                            : 'Optional. Add paths only when the command rule should apply to a specific file, directory, URL, or other resource.',
                                                    columnLabel: 'Path / Resource',
                                                    value: editorState.resources,
                                                    oversizedField: oversizedListFields.resources,
                                                    selectedIndex: selectedResourceRow,
                                                    onSelectedIndexChange: setSelectedResourceRow,
                                                    onChange: (resources) => setEditorState((state) => ({ ...state, resources })),
                                                    placeholder: '~/.ssh',
                                                    helperText: 'Optional. Add one resource per row.',
                                                })}
                                                <FormControl size="small" fullWidth>
                                                    <InputLabel id="resource-mode">Resource Match</InputLabel>
                                                    <Select
                                                        labelId="resource-mode"
                                                        label="Resource Match"
                                                        value={editorState.resourceMode}
                                                        onChange={(e) => setEditorState((state) => ({ ...state, resourceMode: String(e.target.value) }))}
                                                    >
                                                        <MenuItem value="prefix">prefix</MenuItem>
                                                        <MenuItem value="contains">contains</MenuItem>
                                                        <MenuItem value="exact">exact</MenuItem>
                                                    </Select>
                                                    <FormHelperText>
                                                        This match mode currently applies to every resource in the list. Use a resource filter only when command matching alone is too broad.
                                                    </FormHelperText>
                                                </FormControl>
                                            </Stack>
                                        </Box>
                                    </Box>
                                ) : (
                                    <Box
                                        sx={{
                                            display: 'grid',
                                            gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                                            gap: 2,
                                        }}
                                    >
                                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, gridColumn: { md: '1 / span 2' } }}>
                                            <Stack spacing={1.5}>
                                                {renderCompactListEditor({
                                                    title: 'Content Patterns',
                                                    description: 'Define the text you want to block or review. Each row becomes one pattern.',
                                                    columnLabel: 'Pattern',
                                                    value: editorState.patterns,
                                                    oversizedField: oversizedListFields.patterns,
                                                    selectedIndex: selectedPatternRow,
                                                    onSelectedIndexChange: setSelectedPatternRow,
                                                    onChange: (patterns) => setEditorState((state) => ({ ...state, patterns })),
                                                    placeholder: 'BEGIN OPENSSH PRIVATE KEY',
                                                    helperText: 'Use a few specific patterns instead of a long generic list.',
                                                })}
                                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                                                    <FormControl size="small" fullWidth>
                                                        <InputLabel id="pattern-mode">Pattern Mode</InputLabel>
                                                        <Select
                                                            labelId="pattern-mode"
                                                            label="Pattern Mode"
                                                            value={editorState.patternMode}
                                                            onChange={(e) => setEditorState((state) => ({ ...state, patternMode: String(e.target.value) }))}
                                                        >
                                                            <MenuItem value="substring">substring</MenuItem>
                                                            <MenuItem value="regex">regex</MenuItem>
                                                        </Select>
                                                        <FormHelperText>Use regex only when substring matching is not precise enough.</FormHelperText>
                                                    </FormControl>
                                                    <FormControlLabel
                                                        sx={{ ml: 0, alignItems: 'center', minWidth: { md: 160 } }}
                                                        control={
                                                            <Switch
                                                                size="small"
                                                                checked={editorState.caseSensitive}
                                                                onChange={(e) => setEditorState((state) => ({ ...state, caseSensitive: e.target.checked }))}
                                                            />
                                                        }
                                                        label="Case sensitive"
                                                    />
                                                </Stack>
                                            </Stack>
                                        </Box>

                                    </Box>
                                )}

                                <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                                    <Stack spacing={1.5}>
                                        <Stack
                                            direction={{ xs: 'column', md: 'row' }}
                                            spacing={1.5}
                                            justifyContent="space-between"
                                            alignItems={{ xs: 'stretch', md: 'flex-start' }}
                                        >
                                            <Box>
                                                <Typography variant="subtitle2">Reason</Typography>
                                                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                                                    This message is shown when the policy blocks or reviews content. Keep it short, explicit, and user-facing.
                                                </Typography>
                                            </Box>
                                            <Button
                                                variant="outlined"
                                                size="small"
                                                sx={{ minWidth: { md: 140 }, alignSelf: { md: 'flex-start' } }}
                                                onClick={() => setEditorState((state) => ({ ...state, reason: buildSuggestedReason(state) }))}
                                            >
                                                Generate
                                            </Button>
                                        </Stack>
                                        <TextField
                                            size="small"
                                            fullWidth
                                            multiline
                                            minRows={2}
                                            maxRows={4}
                                            value={editorState.reason}
                                            onChange={(e) => setEditorState((state) => ({ ...state, reason: e.target.value }))}
                                            placeholder="Example: Access to protected SSH resources is blocked."
                                        />
                                    </Stack>
                                </Box>

                                <Accordion
                                    expanded={advancedOpen}
                                    onChange={(_, expanded) => setAdvancedOpen(expanded)}
                                    disableGutters
                                    elevation={0}
                                    sx={{
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 2,
                                        '&:before': { display: 'none' },
                                        overflow: 'hidden',
                                    }}
                                >
                                    <AccordionSummary expandIcon={<ExpandMore />}>
                                        <Stack spacing={0.5}>
                                            <Typography variant="subtitle2">Advanced Settings</Typography>
                                            <Typography variant="caption" color="text.secondary">
                                                Review or override the default verdict and scenario scope for this policy.
                                            </Typography>
                                        </Stack>
                                    </AccordionSummary>
                                    <AccordionDetails>
                                        <Stack spacing={2}>
                                            <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                                                <Stack spacing={2}>
                                                    <Box>
                                                        <Typography variant="subtitle2">Set Verdict</Typography>
                                                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                                                            The verdict defines what Guardrails should do once this policy matches.
                                                        </Typography>
                                                        <Box
                                                            sx={{
                                                                display: 'grid',
                                                                gridTemplateColumns: { xs: '1fr', md: '1fr 1fr 1fr' },
                                                                gap: 1.5,
                                                                mt: 1.5,
                                                            }}
                                                        >
                                                            {[
                                                                {
                                                                    value: 'allow',
                                                                    label: 'Allow',
                                                                    description: 'Record the match but allow the content or action to continue.',
                                                                },
                                                                {
                                                                    value: 'review',
                                                                    label: 'Ask',
                                                                    description: 'Reserved for a future interactive verdict. Not selectable yet.',
                                                                    disabled: true,
                                                                },
                                                                {
                                                                    value: 'block',
                                                                    label: 'Block',
                                                                    description: 'Stop the content or action and return the policy reason to the user.',
                                                                },
                                                            ].map((option) => {
                                                                const selected = editorState.verdict === option.value;
                                                                const disabled = Boolean(option.disabled);
                                                                return (
                                                                    <Tooltip key={option.value} title={disabled ? option.description : ''} disableHoverListener={!disabled}>
                                                                        <Box
                                                                            onClick={() => {
                                                                                if (disabled) return;
                                                                                setEditorState((state) => ({ ...state, verdict: option.value }));
                                                                            }}
                                                                            sx={{
                                                                                border: '1px solid',
                                                                                borderColor: selected ? 'primary.main' : 'divider',
                                                                                bgcolor: selected ? 'action.selected' : 'background.paper',
                                                                                borderRadius: 2,
                                                                                p: 1.5,
                                                                                cursor: disabled ? 'not-allowed' : 'pointer',
                                                                                opacity: disabled ? 0.5 : 1,
                                                                                transition: 'all 0.15s ease',
                                                                                '&:hover': disabled ? undefined : { borderColor: 'primary.main', bgcolor: 'action.hover' },
                                                                            }}
                                                                        >
                                                                            <Stack spacing={0.75}>
                                                                                <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                                                                    <Typography variant="body2" fontWeight={600}>
                                                                                        {option.label}
                                                                                    </Typography>
                                                                                    {selected && (
                                                                                        <Tooltip title="Selected">
                                                                                            <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                                                        </Tooltip>
                                                                                    )}
                                                                                </Stack>
                                                                                <Typography variant="caption" color="text.secondary">
                                                                                    {option.description}
                                                                                </Typography>
                                                                            </Stack>
                                                                        </Box>
                                                                    </Tooltip>
                                                                );
                                                            })}
                                                        </Box>
                                                    </Box>
                                                </Stack>
                                            </Box>

                                            {renderScenarioScopeSelector({
                                                title: 'Scenario Scope',
                                                description: 'Select the scenarios where this policy should apply.',
                                                value: editorState.scenarios,
                                                onChange: (scenarios) => setEditorState((state) => ({ ...state, scenarios })),
                                                helperText: 'Policies own their own scope. Groups only control organization and activation.',
                                            })}
                                        </Stack>
                                    </AccordionDetails>
                                </Accordion>
                            </>
                        ) : null}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button variant="text" onClick={handleCloseEditor}>
                        Cancel
                    </Button>
                    <Button variant="outlined" disabled={pendingSave} onClick={handleDuplicatePolicy}>
                        Duplicate
                    </Button>
                    <Button variant="contained" disabled={pendingSave} onClick={handleSavePolicy}>
                        {pendingSave ? 'Saving…' : 'Save'}
                    </Button>
                </DialogActions>
            </Dialog>

            <Dialog open={confirmCloseOpen} onClose={() => handleConfirmClose('cancel')} disableRestoreFocus>
                <DialogTitle>Unsaved changes</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary">
                        You have unsaved changes in this policy. What would you like to do?
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button variant="text" onClick={() => handleConfirmClose('cancel')}>
                        Cancel
                    </Button>
                    <Button variant="outlined" onClick={() => handleConfirmClose('discard')}>
                        Discard
                    </Button>
                    <Button variant="contained" onClick={() => handleConfirmClose('save')}>
                        Save & Close
                    </Button>
                </DialogActions>
            </Dialog>

            <Dialog open={!!deletePolicyId} onClose={() => setDeletePolicyId(null)} disableRestoreFocus>
                <DialogTitle>Delete policy</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary">
                        {deletePolicyId
                            ? `Delete policy "${deletePolicyId}"? This will update the Guardrails config and reload the engine.`
                            : 'Delete this policy?'}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button variant="text" onClick={() => setDeletePolicyId(null)}>
                        Cancel
                    </Button>
                    <Button variant="contained" color="error" onClick={handleDeletePolicy}>
                        Delete
                    </Button>
                </DialogActions>
            </Dialog>

        </PageLayout>
    );
};

export default GuardrailsRulesPage;
