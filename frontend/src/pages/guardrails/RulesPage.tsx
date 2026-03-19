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
    Code as CodeIcon,
    DeleteOutline,
    ExpandMore,
    LaptopMac,
    Refresh as RefreshIcon,
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
    default_verdict?: string;
    default_scope?: {
        scenarios?: string[];
    };
};

type GuardrailsPolicy = {
    id: string;
    name?: string;
    group?: string;
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
        patterns?: string[];
        pattern_mode?: string;
        case_sensitive?: boolean;
    };
    verdict?: string;
    reason?: string;
};

type EditorState = {
    id: string;
    name: string;
    group: string;
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

type GroupEditorState = {
    id: string;
    name: string;
    enabled: boolean;
    severity: string;
    defaultVerdict: string;
    scenarios: string[];
};

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

const GuardrailsRulesPage = () => {
    const location = useLocation();
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [supportedScenarios, setSupportedScenarios] = useState<string[]>([]);
    const [groups, setGroups] = useState<PolicyGroup[]>([]);
    const [policies, setPolicies] = useState<GuardrailsPolicy[]>([]);
    const [pendingPolicyId, setPendingPolicyId] = useState<string | null>(null);
    const [pendingSave, setPendingSave] = useState(false);
    const [selectedPolicyId, setSelectedPolicyId] = useState<string | null>(null);
    const [isNewPolicy, setIsNewPolicy] = useState(false);
    const [editorOpen, setEditorOpen] = useState(false);
    const [editorSnapshot, setEditorSnapshot] = useState('');
    const [confirmCloseOpen, setConfirmCloseOpen] = useState(false);
    const [deletePolicyId, setDeletePolicyId] = useState<string | null>(null);
    const [groupDialogOpen, setGroupDialogOpen] = useState(false);
    const [editingGroupId, setEditingGroupId] = useState<string | null>(null);
    const [deleteGroupId, setDeleteGroupId] = useState<string | null>(null);
    const [pendingGroupId, setPendingGroupId] = useState<string | null>(null);
    const [pendingGroupSave, setPendingGroupSave] = useState(false);
    const [initializingDefaultGroup, setInitializingDefaultGroup] = useState(false);
    const [advancedOpen, setAdvancedOpen] = useState(false);
    const [selectedPolicyTab, setSelectedPolicyTab] = useState<'resource_access' | 'command_execution' | 'content'>(
        'resource_access'
    );
    const [selectedResourceRow, setSelectedResourceRow] = useState(-1);
    const [selectedCommandTermRow, setSelectedCommandTermRow] = useState(-1);
    const [selectedPatternRow, setSelectedPatternRow] = useState(-1);
    const [editorState, setEditorState] = useState<EditorState>({
        id: '',
        name: '',
        group: '',
        kind: '',
        enabled: true,
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
    const [groupEditorState, setGroupEditorState] = useState<GroupEditorState>({
        id: '',
        name: '',
        enabled: true,
        severity: 'medium',
        defaultVerdict: 'block',
        scenarios: [],
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
    const normalizeGroup = (value?: string) => value?.trim() || '';

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

    const isEditorDirty = useMemo(() => {
        if (!editorSnapshot) {
            return false;
        }
        return JSON.stringify(editorState) !== editorSnapshot;
    }, [editorState, editorSnapshot]);

    const scenarioOptions = useMemo(() => supportedScenarios.filter(Boolean), [supportedScenarios]);
    const groupOptions = useMemo(
        () => groups.map((group) => ({ value: group.id, label: group.name || group.id })),
        [groups]
    );
    const resourceAccessPolicies = useMemo(
        () => policies.filter((policy) => policy.kind === 'resource_access' || policy.kind === 'operation'),
        [policies]
    );
    const commandExecutionPolicies = useMemo(
        () => policies.filter((policy) => policy.kind === 'command_execution'),
        [policies]
    );
    const contentPolicies = useMemo(
        () => policies.filter((policy) => policy.kind === 'content'),
        [policies]
    );

    const generatePolicyId = (name: string, kind: EditorState['kind'], currentId?: string) => {
        const normalizedName = name
            .toLowerCase()
            .trim()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '');
        const prefix = kind ? `${kind}-` : '';
        const baseId = `${prefix}${normalizedName || 'policy'}`;
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

    const generateGroupId = (name: string, currentId?: string) => {
        const normalizedName = name
            .toLowerCase()
            .trim()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-+|-+$/g, '');
        const baseId = normalizedName || 'group';
        const existingIds = new Set(groups.map((group) => group.id).filter((groupId) => groupId && groupId !== currentId));

        let candidate = baseId;
        let suffix = 2;
        while (existingIds.has(candidate)) {
            candidate = `${baseId}-${suffix}`;
            suffix += 1;
        }
        return candidate;
    };

    const applyKindDefaults = (kind: 'resource_access' | 'command_execution' | 'content', current: EditorState): EditorState => {
        const nextName =
            current.name && current.name !== 'New Policy'
                ? current.name
                : kind === 'resource_access'
                  ? 'New Resource Access Policy'
                  : kind === 'command_execution'
                    ? 'New Command Execution Policy'
                    : 'New Content Policy';

        return {
            ...current,
            kind,
            name: nextName,
            id: isNewPolicy ? generatePolicyId(nextName, kind) : current.id,
            toolNames: kind === 'content' ? '' : current.toolNames || 'bash',
            actions:
                kind === 'resource_access'
                    ? current.actions.length > 0
                        ? current.actions.filter((action) => action !== 'execute')
                        : ['read']
                    : kind === 'command_execution'
                      ? ['execute']
                      : [],
            commandTerms: kind === 'command_execution' ? current.commandTerms : '',
            patterns: kind === 'content' ? current.patterns : '',
        };
    };

    const buildSuggestedReason = (state: EditorState) => {
        if (state.kind === 'command_execution') {
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

    const buildPolicySummary = (policy: GuardrailsPolicy) => {
        if (policy.kind === 'command_execution') {
            const terms = policy.match?.terms?.join(', ') || 'any command';
            const resources = policy.match?.resources?.values?.join(', ');
            const toolNames = policy.match?.tool_names?.join(', ') || 'any tool';
            return resources ? `${toolNames} · execute · ${terms} · ${resources}` : `${toolNames} · execute · ${terms}`;
        }
        if (policy.kind === 'resource_access' || policy.kind === 'operation') {
            const actions = policy.match?.actions?.include?.join(', ') || 'any action';
            const resources = policy.match?.resources?.values?.join(', ') || 'any resource';
            const toolNames = policy.match?.tool_names?.join(', ') || 'any tool';
            return `${toolNames} · ${actions} · ${resources}`;
        }
        const patterns = policy.match?.patterns || [];
        if (patterns.length === 0) {
            return 'No patterns configured';
        }
        return patterns.slice(0, 2).join(', ');
    };

    const buildPolicyScope = (policy: GuardrailsPolicy) => {
        const scenarios = policy.scope?.scenarios?.join(', ') || 'all scenarios';
        return scenarios;
    };

    const buildGroupSummary = (group: PolicyGroup) => {
        const severity = group.severity || 'medium';
        const verdict = group.default_verdict || 'block';
        const scenarios = group.default_scope?.scenarios?.join(', ') || 'all scenarios';
        return `${severity} · ${verdict} · ${scenarios}`;
    };

    const getGroupByID = (groupID?: string) => groups.find((group) => group.id === groupID);

    const getGroupDefaultScenarios = (group?: PolicyGroup) => {
        const scenarios = group?.default_scope?.scenarios;
        return scenarios && scenarios.length > 0 ? scenarios : scenarioOptions;
    };

    const getGroupDefaultVerdict = (group?: PolicyGroup) => group?.default_verdict || 'block';

    // MUI restores focus to the trigger after a dialog closes. Blur it so toolbar buttons
    // do not keep the white focus overlay after closing policy/group dialogs.
    const blurActiveElement = () => {
        const active = document.activeElement;
        if (active instanceof HTMLElement) {
            active.blur();
        }
    };

    const makeEditorState = (policy?: GuardrailsPolicy): EditorState => {
        const group = policy?.group || '';
        const selectedGroup = getGroupByID(group);
        const scenarios =
            policy?.scope?.scenarios && policy.scope.scenarios.length > 0
                ? policy.scope.scenarios
                : getGroupDefaultScenarios(selectedGroup);
        const nextState: EditorState = {
            id: policy?.id || '',
            name: policy?.name || '',
            group,
            kind: policy?.kind === 'operation' ? 'resource_access' : policy?.kind || '',
            enabled: policy?.enabled !== false,
            verdict: policy?.verdict || getGroupDefaultVerdict(selectedGroup),
            scenarios,
            toolNames: joinLines(policy?.match?.tool_names),
            actions: policy?.match?.actions?.include || [],
            commandTerms: joinLines(policy?.match?.terms),
            resources: joinLines(policy?.match?.resources?.values),
            resourceMode: policy?.match?.resources?.mode || 'prefix',
            patterns: joinLines(policy?.match?.patterns),
            patternMode: policy?.match?.pattern_mode || 'substring',
            caseSensitive: !!policy?.match?.case_sensitive,
            reason: policy?.reason || '',
        };
        return nextState;
    };

    const makeEditorStateFromDraft = (draft: Partial<EditorState>): EditorState => {
        const baseState = makeEditorState();
        const selectedGroup = getGroupByID(normalizeGroup(draft.group));
        return {
            ...baseState,
            ...draft,
            id: draft.id || baseState.id,
            name: draft.name || baseState.name,
            group: normalizeGroup(draft.group),
            kind: draft.kind || baseState.kind,
            enabled: draft.enabled ?? baseState.enabled,
            verdict: draft.verdict || getGroupDefaultVerdict(selectedGroup) || baseState.verdict,
            scenarios:
                draft.scenarios && draft.scenarios.length > 0
                    ? draft.scenarios
                    : getGroupDefaultScenarios(selectedGroup),
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

    const makeGroupEditorState = (group?: PolicyGroup): GroupEditorState => ({
        id: group?.id || '',
        name: group?.name || '',
        enabled: group?.enabled !== false,
        severity: group?.severity || 'medium',
        defaultVerdict: group?.default_verdict || 'block',
        scenarios:
            group?.default_scope?.scenarios && group.default_scope.scenarios.length > 0
                ? group.default_scope.scenarios
                : scenarioOptions,
    });

    const loadPolicies = async (silent = false) => {
        try {
            if (!silent) {
                setLoading(true);
            }
            const guardrailsConfig = await api.getGuardrailsConfig();
            const config = guardrailsConfig?.config || {};
            const scenarios = Array.isArray(guardrailsConfig?.supported_scenarios)
                ? guardrailsConfig.supported_scenarios.filter((value: string) => value && value !== '_global')
                : [];
            setSupportedScenarios(scenarios);
            setGroups(Array.isArray(config.groups) ? config.groups : []);
            setPolicies(Array.isArray(config.policies) ? config.policies : []);
            setLoadError(null);
        } catch (error) {
            console.error('Failed to load guardrails config:', error);
            setGroups([]);
            setPolicies([]);
            setSupportedScenarios([]);
            setLoadError('Failed to load guardrails config');
        } finally {
            if (!silent) {
                setLoading(false);
            }
        }
    };

    useEffect(() => {
        loadPolicies();
    }, []);

    useEffect(() => {
        if (loading || loadError || initializingDefaultGroup || groups.length > 0 || supportedScenarios.length === 0) {
            return;
        }

        const ensureDefaultGroup = async () => {
            try {
                setInitializingDefaultGroup(true);
                const result = await api.createGuardrailsGroup({
                    id: 'default',
                    name: 'Default',
                    enabled: true,
                    severity: 'high',
                    default_verdict: 'block',
                    default_scope: {
                        scenarios: supportedScenarios,
                    },
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
    }, [groups.length, initializingDefaultGroup, loadError, loading, supportedScenarios]);

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
        const nextState = makeEditorState(policy);
        setSelectedPolicyId(policy.id);
        setIsNewPolicy(false);
        setEditorOpen(true);
        setAdvancedOpen(nextState.group === '');
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(nextState));
        navigate('/guardrails/rules', { replace: true });
    }, [location.search, navigate, policies, scenarioOptions]);

    useEffect(() => {
        const draft = (location.state as { newPolicyDraft?: Partial<EditorState> } | null)?.newPolicyDraft;
        if (!draft) {
            return;
        }
        const nextState = makeEditorStateFromDraft(draft);
        setSelectedPolicyId(null);
        setIsNewPolicy(true);
        setEditorOpen(true);
        setAdvancedOpen(nextState.group === '');
        setSelectedResourceRow(splitLines(nextState.resources).length > 0 ? 0 : -1);
        setSelectedCommandTermRow(splitLines(nextState.commandTerms).length > 0 ? 0 : -1);
        setSelectedPatternRow(splitLines(nextState.patterns).length > 0 ? 0 : -1);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(nextState));
        navigate('/guardrails/rules', { replace: true, state: null });
    }, [location.state, navigate, scenarioOptions]);

    const handleReload = async () => {
        try {
            setLoading(true);
            const result = await api.reloadGuardrailsConfig();
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to reload guardrails config' });
                return;
            }
            await loadPolicies(true);
            setActionMessage({ type: 'success', text: 'Guardrails reloaded successfully.' });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to reload guardrails config' });
        } finally {
            setLoading(false);
        }
    };

    const openNewGroupDialog = () => {
        blurActiveElement();
        setEditingGroupId(null);
        setGroupEditorState(makeGroupEditorState());
        setGroupDialogOpen(true);
    };

    const openEditGroupDialog = (group: PolicyGroup) => {
        blurActiveElement();
        setEditingGroupId(group.id);
        setGroupEditorState(makeGroupEditorState(group));
        setGroupDialogOpen(true);
    };

    const handleSaveGroup = async () => {
        if (!groupEditorState.name.trim()) {
            setActionMessage({ type: 'error', text: 'Group name is required before saving.' });
            return;
        }
        if (!groupEditorState.id.trim()) {
            setActionMessage({ type: 'error', text: 'Group ID could not be generated.' });
            return;
        }

        const payload = {
            id: groupEditorState.id,
            name: groupEditorState.name,
            enabled: groupEditorState.enabled,
            severity: groupEditorState.severity,
            default_verdict: groupEditorState.defaultVerdict,
            default_scope: {
                scenarios: groupEditorState.scenarios,
            },
        };

        try {
            setPendingGroupSave(true);
            const result = editingGroupId
                ? await api.updateGuardrailsGroup(editingGroupId, payload)
                : await api.createGuardrailsGroup(payload);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to save group' });
                return;
            }
            await loadPolicies(true);
            if (editingGroupId && editingGroupId !== groupEditorState.id && editorState.group === editingGroupId) {
                setEditorState((state) => ({ ...state, group: groupEditorState.id }));
            }
            setGroupDialogOpen(false);
            blurActiveElement();
            setActionMessage({ type: 'success', text: `Group "${groupEditorState.id}" saved.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to save group' });
        } finally {
            setPendingGroupSave(false);
        }
    };

    const handleDeleteGroup = async () => {
        if (!deleteGroupId) {
            return;
        }
        try {
            setPendingGroupId(deleteGroupId);
            const result = await api.deleteGuardrailsGroup(deleteGroupId);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to delete group' });
                return;
            }
            await loadPolicies(true);
            if (editorState.group === deleteGroupId) {
                setEditorState((state) => ({ ...state, group: '' }));
            }
            setDeleteGroupId(null);
            setActionMessage({ type: 'success', text: `Group "${deleteGroupId}" deleted.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to delete group' });
        } finally {
            setPendingGroupId(null);
        }
    };

    const handleToggleGroup = async (groupId: string, enabled: boolean) => {
        const group = groups.find((item) => item.id === groupId);
        if (!group) {
            return;
        }

        const payload = {
            id: group.id,
            name: group.name || group.id,
            enabled,
            severity: group.severity || 'medium',
            default_verdict: group.default_verdict || 'block',
            default_scope: {
                scenarios: group.default_scope?.scenarios || [],
            },
        };

        try {
            setPendingGroupId(groupId);
            const result = await api.updateGuardrailsGroup(groupId, payload);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update group' });
                return;
            }
            await loadPolicies(true);
            setActionMessage({ type: 'success', text: `Group "${groupId}" updated.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to update group' });
        } finally {
            setPendingGroupId(null);
        }
    };

    const openPolicyEditor = (policy: GuardrailsPolicy) => {
        const nextState = makeEditorState(policy);
        setSelectedPolicyId(policy.id);
        setIsNewPolicy(false);
        setEditorOpen(true);
        setAdvancedOpen(nextState.group === '');
        setSelectedResourceRow(splitLines(nextState.resources).length > 0 ? 0 : -1);
        setSelectedCommandTermRow(splitLines(nextState.commandTerms).length > 0 ? 0 : -1);
        setSelectedPatternRow(splitLines(nextState.patterns).length > 0 ? 0 : -1);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(nextState));
    };

    const handleNewPolicy = (kind?: 'resource_access' | 'command_execution' | 'content') => {
        const baseState = makeEditorState();
        const nextState = kind ? applyKindDefaults(kind, baseState) : baseState;
        setSelectedPolicyId(null);
        setIsNewPolicy(true);
        setEditorOpen(true);
        setAdvancedOpen(nextState.group === '');
        setSelectedResourceRow(splitLines(nextState.resources).length > 0 ? 0 : -1);
        setSelectedCommandTermRow(splitLines(nextState.commandTerms).length > 0 ? 0 : -1);
        setSelectedPatternRow(splitLines(nextState.patterns).length > 0 ? 0 : -1);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(nextState));
    };

    const handleSelectPolicyGroup = (groupID: string) => {
        const selectedGroup = getGroupByID(groupID);
        setEditorState((state) => ({
            ...state,
            group: groupID,
            verdict: groupID ? getGroupDefaultVerdict(selectedGroup) : state.verdict,
            scenarios: groupID ? getGroupDefaultScenarios(selectedGroup) : state.scenarios,
        }));
        setAdvancedOpen(groupID === '');
    };

    const buildPolicyPayload = (state: EditorState) => {
        const operationMatch = {
            tool_names: splitLines(state.toolNames),
            actions: {
                include:
                    state.kind === 'command_execution'
                        ? ['execute']
                        : state.actions.filter((action) => action !== 'execute'),
            },
            terms: state.kind === 'command_execution' ? splitLines(state.commandTerms) : [],
            resources: {
                type: 'path',
                mode: state.resourceMode,
                values: splitLines(state.resources),
            },
        };
        const payload = {
            id: state.id,
            name: state.name,
            group: state.group,
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
                          patterns: splitLines(state.patterns),
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
        if (!editorState.id.trim()) {
            setActionMessage({ type: 'error', text: 'Policy name is required before saving.' });
            return false;
        }
        if (editorState.kind === 'content' && splitLines(editorState.patterns).length === 0) {
            setActionMessage({ type: 'error', text: 'Content policies require at least one pattern.' });
            return false;
        }
        if (
            editorState.kind === 'resource_access' &&
            splitLines(editorState.resources).length === 0 &&
            editorState.actions.length === 0 &&
            splitLines(editorState.toolNames).length === 0
        ) {
            setActionMessage({ type: 'error', text: 'Resource access policies require at least one action, resource, or tool filter.' });
            return false;
        }
        if (
            editorState.kind === 'command_execution' &&
            splitLines(editorState.commandTerms).length === 0 &&
            splitLines(editorState.toolNames).length === 0 &&
            splitLines(editorState.resources).length === 0
        ) {
            setActionMessage({ type: 'error', text: 'Command execution policies require a command match, tool filter, or resource filter.' });
            return false;
        }

        try {
            setPendingSave(true);
            const payload = buildPolicyPayload(editorState);
            const targetPolicyId = isNewPolicy ? editorState.id : (selectedPolicyId || editorState.id);
            const result = isNewPolicy
                ? await api.createGuardrailsPolicy(payload)
                : await api.updateGuardrailsPolicy(targetPolicyId, payload);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to save policy' });
                return false;
            }
            await loadPolicies(true);
            setSelectedPolicyId(editorState.id);
            setIsNewPolicy(false);
            setEditorSnapshot(JSON.stringify(editorState));
            setActionMessage({ type: 'success', text: `Policy "${editorState.id}" saved.` });
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
            const result = await api.updateGuardrailsPolicy(policyId, { enabled });
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update policy' });
                return;
            }
            await loadPolicies(true);
            if (selectedPolicyId === policyId) {
                setEditorState((state) => ({ ...state, enabled }));
                setEditorSnapshot((snapshot) => {
                    if (!snapshot) {
                        return snapshot;
                    }
                    const nextSnapshot = JSON.parse(snapshot);
                    nextSnapshot.enabled = enabled;
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
        items: GuardrailsPolicy[],
        kind: 'resource_access' | 'command_execution' | 'content'
    ) => (
        <Box sx={{ mb: 3 }}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1.5 }}>
                <Typography variant="subtitle1" fontWeight={500}>
                    {title}
                </Typography>
                <Chip
                    label={items.length}
                    size="small"
                    color="primary"
                    variant="outlined"
                    sx={{ height: 20, minWidth: 20, fontSize: '0.7rem' }}
                />
            </Stack>
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
                                  : 'No content policies yet'
                        }
                        description={
                            kind === 'resource_access'
                                ? 'Start with a guided resource access policy to control reads, writes, deletes, and protected paths.'
                                : kind === 'command_execution'
                                  ? 'Start with a guided command execution policy to control dangerous or disallowed commands.'
                                  : 'Start with a guided content policy to filter model output or tool results.'
                        }
                        showOAuthButton={false}
                        showHeroIcon={false}
                        primaryButtonLabel="New Policy"
                        onAddApiKeyClick={() => handleNewPolicy(kind)}
                    />
                </Box>
            ) : (
                <List dense sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, py: 0 }}>
                    {items.map((policy) => (
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
                                    alignItems: { xs: 'flex-start', lg: 'center' },
                                    flexDirection: { xs: 'column', lg: 'row' },
                                    gap: 1.5,
                                    width: '100%',
                                    cursor: 'pointer',
                                    px: 2,
                                    py: 1.5,
                                    bgcolor: selectedPolicyId === policy.id ? 'action.selected' : 'transparent',
                                    '&:hover': { bgcolor: 'action.hover' },
                                }}
                                onClick={() => openPolicyEditor(policy)}
                            >
                                <Box sx={{ minWidth: { lg: 220 }, flexShrink: 0 }}>
                                    <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                            {policy.id}
                                        </Typography>
                                        {policy.group && <Chip size="small" label={policy.group} variant="outlined" />}
                                    </Stack>
                                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                                        {policy.name || 'Unnamed policy'}
                                    </Typography>
                                </Box>

                                <Box sx={{ flex: 1, minWidth: 0 }}>
                                    <Typography variant="body2" color="text.primary" sx={{ whiteSpace: 'normal' }}>
                                        {buildPolicySummary(policy)}
                                    </Typography>
                                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5, whiteSpace: 'normal' }}>
                                        {buildPolicyScope(policy)}
                                    </Typography>
                                </Box>

                                <Stack
                                    direction={{ xs: 'row', sm: 'row' }}
                                    spacing={1}
                                    alignItems="center"
                                    sx={{ width: { xs: '100%', lg: 'auto' }, justifyContent: { xs: 'space-between', lg: 'flex-end' } }}
                                >
                                    <Chip size="small" label={policy.enabled === false ? 'Disabled' : 'Enabled'} />
                                    <FormControlLabel
                                        sx={{ ml: 0 }}
                                        onClick={(e) => e.stopPropagation()}
                                        control={
                                            <Switch
                                                size="small"
                                                checked={policy.enabled !== false}
                                                disabled={pendingPolicyId === policy.id}
                                                onChange={(e) => handleTogglePolicy(policy.id, e.target.checked)}
                                            />
                                        }
                                        label="Enabled"
                                    />
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
                                </Stack>
                            </Box>
                        </ListItem>
                    ))}
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
    }) => {
        const rows = textListRows(value);
        const isEmpty = rows.length === 1 && rows[0] === '';
        const showEmptyState = isEmpty && selectedIndex < 0;
        const canRemove = !showEmptyState;
        const visibleRows = showEmptyState ? [] : rows;

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
                        sx={{
                            px: 1,
                            py: 0.5,
                            borderBottom: '1px solid',
                            borderColor: 'divider',
                            bgcolor: 'action.hover',
                        }}
                    >
                        <IconButton
                            size="small"
                            color="primary"
                            onClick={() => {
                                if (showEmptyState) {
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
                <FormHelperText>{helperText}</FormHelperText>
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
                                        {selected && <Chip size="small" color="primary" label="Selected" />}
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
                    title="Guardrails Policies"
                    subtitle="Manage policy-based Guardrails configuration. Policies are grouped by risk or ownership, then compiled into the runtime engine."
                    size="full"
                    rightAction={
                        <Tooltip title="Reload guardrails config">
                            <IconButton onClick={handleReload} size="small" aria-label="Reload guardrails config">
                                <RefreshIcon />
                            </IconButton>
                        </Tooltip>
                    }
                >
                    <Stack spacing={1.5}>
                        <Typography variant="body2" color="text.secondary">
                            Use resource access policies to protect files and directories, command execution policies to control risky commands, and content policies to filter model or tool output.
                        </Typography>
                        {loadError && <Alert severity="error">{loadError}</Alert>}
                        {actionMessage && <Alert severity={actionMessage.type}>{actionMessage.text}</Alert>}
                        <Divider />
                        <Stack spacing={1.5}>
                            <Stack direction="row" spacing={1} alignItems="center" justifyContent="space-between">
                                <Typography variant="subtitle2">Groups</Typography>
                                <Button size="small" variant="contained" onClick={openNewGroupDialog}>
                                    New Group
                                </Button>
                            </Stack>
                            <Typography variant="body2" color="text.secondary">
                                Groups are a way to organize policies and share defaults. A group can define things like severity,
                                default verdict, and default scope, so policies inside the group do not need to repeat those
                                settings.
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                Use groups to separate high-risk policies from lower-risk ones, or to organize policies by team,
                                scenario, or use case.
                            </Typography>
                            <Typography variant="caption" color="text.secondary">
                                {groups.length} group{groups.length === 1 ? '' : 's'} configured.
                            </Typography>

                            {groups.length === 0 ? (
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <Chip size="small" label="No groups defined" variant="outlined" />
                                    <Button size="small" variant="contained" onClick={openNewGroupDialog}>
                                        New Group
                                    </Button>
                                </Stack>
                            ) : (
                                <List dense sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, py: 0 }}>
                                    {groups.map((group) => (
                                        <ListItem
                                            key={group.id}
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
                                                    alignItems: { xs: 'flex-start', md: 'center' },
                                                    flexDirection: { xs: 'column', md: 'row' },
                                                    gap: 1.5,
                                                    width: '100%',
                                                    px: 2,
                                                    py: 1.5,
                                                    cursor: 'pointer',
                                                    '&:hover': { bgcolor: 'action.hover' },
                                                }}
                                                onClick={() => openEditGroupDialog(group)}
                                            >
                                                <Box sx={{ minWidth: { md: 220 }, flexShrink: 0 }}>
                                                    <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                            {group.name || group.id}
                                                        </Typography>
                                                        <Chip size="small" label={group.id} variant="outlined" />
                                                    </Stack>
                                                </Box>

                                                <Box sx={{ flex: 1, minWidth: 0 }}>
                                                    <Typography variant="body2" color="text.primary" sx={{ whiteSpace: 'normal' }}>
                                                        {buildGroupSummary(group)}
                                                    </Typography>
                                                </Box>

                                                <Stack
                                                    direction="row"
                                                    spacing={1}
                                                    alignItems="center"
                                                    sx={{ width: { xs: '100%', md: 'auto' }, justifyContent: { xs: 'space-between', md: 'flex-end' } }}
                                                >
                                                    <Chip size="small" label={group.enabled === false ? 'Disabled' : 'Enabled'} />
                                                    <FormControlLabel
                                                        sx={{ ml: 0 }}
                                                        onClick={(e) => e.stopPropagation()}
                                                        control={
                                                            <Switch
                                                                size="small"
                                                                checked={group.enabled !== false}
                                                                disabled={pendingGroupId === group.id}
                                                                onChange={(e) => handleToggleGroup(group.id, e.target.checked)}
                                                            />
                                                        }
                                                        label="Enabled"
                                                    />
                                                    <Tooltip title="Delete group" arrow>
                                                        <span>
                                                            <IconButton
                                                                size="small"
                                                                disabled={pendingGroupId === group.id}
                                                                onClick={(e) => {
                                                                    e.stopPropagation();
                                                                    setDeleteGroupId(group.id);
                                                                }}
                                                            >
                                                                <DeleteOutline fontSize="small" />
                                                            </IconButton>
                                                        </span>
                                                    </Tooltip>
                                                </Stack>
                                            </Box>
                                        </ListItem>
                                    ))}
                                </List>
                            )}
                        </Stack>
                    </Stack>
                </UnifiedCard>

                <UnifiedCard
                    title="Policies"
                    subtitle={`${policies.length} polic${policies.length === 1 ? 'y' : 'ies'} configured`}
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
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
                        <Alert severity="info">
                            Create or edit policies in a dialog. Resource access policies protect files and directories, command execution policies control commands, and content policies filter returned text.
                        </Alert>
                        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
                            <Tabs
                                value={selectedPolicyTab}
                                onChange={(_, value) => setSelectedPolicyTab(value)}
                                variant="scrollable"
                                scrollButtons="auto"
                            >
                                <Tab value="resource_access" label={`Resource Access (${resourceAccessPolicies.length})`} />
                                <Tab value="command_execution" label={`Command Execution (${commandExecutionPolicies.length})`} />
                                <Tab value="content" label={`Content (${contentPolicies.length})`} />
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
                                'Content Policies',
                                'Use these to filter model output and tool results before they are shown or forwarded.',
                                contentPolicies,
                                'content'
                            )}
                    </Stack>
                </UnifiedCard>
            </Stack>

            <Dialog open={editorOpen} onClose={handleCloseEditor} disableRestoreFocus fullWidth maxWidth="md">
                <DialogTitle>{isNewPolicy ? 'New Policy' : `Edit Policy${selectedPolicyId ? ` · ${selectedPolicyId}` : ''}`}</DialogTitle>
                <DialogContent dividers>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2">Policy Editor</Typography>
                            <Stack spacing={0} alignItems="flex-end">
                                <FormControlLabel
                                    control={
                                        <Switch
                                            size="small"
                                            checked={editorState.enabled}
                                            onChange={(e) => setEditorState((state) => ({ ...state, enabled: e.target.checked }))}
                                        />
                                    }
                                    label="Enabled"
                                />
                                <Typography variant="caption" color="text.secondary">
                                    Synced with the list toggle.
                                </Typography>
                            </Stack>
                        </Box>

                        <Alert severity="info">
                            {editorState.kind === 'resource_access'
                                ? 'Guided flow: choose the access action first, then define the protected resource.'
                                : editorState.kind === 'command_execution'
                                  ? 'Guided flow: describe the command you want to block or review, then add optional tool or resource filters.'
                                : editorState.kind === 'content'
                                  ? 'Guided flow: choose where the content appears, then add a few patterns that clearly describe what should be blocked or reviewed.'
                                  : 'Start by choosing the policy kind. The editor will then show the relevant options.'}
                        </Alert>

                        <Typography variant="subtitle2">Basic Settings</Typography>
                        <Stack spacing={1}>
                            <Typography variant="body2" color="text.secondary">
                                Policy kind defines what the policy inspects. Choose one before filling the rest of the form.
                            </Typography>
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
                                    <Stack spacing={1.25}>
                                        <Stack direction="row" spacing={1} alignItems="center">
                                            <Terminal fontSize="small" color={editorState.kind === 'resource_access' ? 'primary' : 'action'} />
                                            <Typography variant="subtitle2">Resource Access</Typography>
                                            {editorState.kind === 'resource_access' && <Chip size="small" color="primary" label="Selected" />}
                                        </Stack>
                                        <Typography variant="body2" color="text.secondary">
                                            Inspect access to files, directories, and other resources. Use this for reads, writes, deletes, and other resource-oriented operations.
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            Best for: `~/.ssh`, `.env`, secrets files, protected paths.
                                        </Typography>
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
                                        <Stack spacing={1.25}>
                                        <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                            <Terminal fontSize="small" color={editorState.kind === 'command_execution' ? 'primary' : 'action'} />
                                            <Typography variant="subtitle2" sx={{ whiteSpace: 'nowrap' }}>
                                                Command Execution
                                            </Typography>
                                            {editorState.kind === 'command_execution' && <Chip size="small" color="primary" label="Selected" />}
                                        </Stack>
                                        <Typography variant="body2" color="text.secondary">
                                            Inspect commands that the model wants to run. Use this for dangerous shell commands, execution patterns, or risky programs.
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            Best for: `rm -rf`, `curl | sh`, shell execution, dangerous commands.
                                        </Typography>
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
                                    <Stack spacing={1.25}>
                                        <Stack direction="row" spacing={1} alignItems="center">
                                            <ArticleOutlined fontSize="small" color={editorState.kind === 'content' ? 'primary' : 'action'} />
                                            <Typography variant="subtitle2">Content Policy</Typography>
                                            {editorState.kind === 'content' && <Chip size="small" color="primary" label="Selected" />}
                                        </Stack>
                                        <Typography variant="body2" color="text.secondary">
                                            Inspect returned text from the model or tools. Use this for secrets, unsafe phrases, sensitive file contents, or outputs that should be blocked.
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            Best for: tool results, model output, text patterns, secret filtering.
                                        </Typography>
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
                                helperText="Human-friendly label shown in UI."
                                disabled={!editorState.kind}
                            />
                        </Stack>

                        {editorState.kind ? (
                            <>
                                <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                                    <Stack spacing={2}>
                                        <Box>
                                            <Typography variant="subtitle2">
                                                Assign Group
                                            </Typography>
                                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                                                Groups are mainly for ownership and shared defaults. Choose one when this policy belongs to a specific risk bucket, team, or scenario set.
                                            </Typography>
                                            <Box
                                                sx={{
                                                    display: 'grid',
                                                    gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' },
                                                    gap: 1.5,
                                                    mt: 1.5,
                                                }}
                                            >
                                                <Box
                                                    onClick={() => handleSelectPolicyGroup('')}
                                                    sx={{
                                                        border: '1px solid',
                                                        borderColor: editorState.group === '' ? 'primary.main' : 'divider',
                                                        bgcolor: editorState.group === '' ? 'action.selected' : 'background.paper',
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
                                                                Ungrouped
                                                            </Typography>
                                                            {editorState.group === '' && <Chip size="small" color="primary" label="Selected" />}
                                                        </Stack>
                                                        <Typography variant="caption" color="text.secondary">
                                                            Keep this policy standalone. It will not inherit any group defaults.
                                                        </Typography>
                                                    </Stack>
                                                </Box>
                                                {groupOptions.map((option) => {
                                                    const selected = editorState.group === option.value;
                                                    return (
                                                        <Box
                                                            key={option.value}
                                                            onClick={() => handleSelectPolicyGroup(option.value)}
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
                                                                    {selected && <Chip size="small" color="primary" label="Selected" />}
                                                                </Stack>
                                                                <Typography variant="caption" color="text.secondary">
                                                                    Policies in this group inherit its defaults when those fields are left empty.
                                                                </Typography>
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
                                                <Typography variant="subtitle2">Choose Actions</Typography>
                                                <Typography variant="caption" color="text.secondary">
                                                    Choose the type of resource access you want to control. These actions focus on files, directories, and other protected paths.
                                                </Typography>
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
                                                                        {selected && <Chip size="small" color="primary" label="Selected" />}
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
                                                    `Command Execution` policies always use `execute`, so `execute` is not shown here. Shell redirection is treated as `write`.
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
                                            {renderCompactListEditor({
                                                title: 'Command Match',
                                                description: 'Describe the command patterns you want to block or review. This is the main selector for execution policies.',
                                                columnLabel: 'Command Pattern',
                                                value: editorState.commandTerms,
                                                selectedIndex: selectedCommandTermRow,
                                                onSelectedIndexChange: setSelectedCommandTermRow,
                                                onChange: (commandTerms) => setEditorState((state) => ({ ...state, commandTerms })),
                                                placeholder: 'rm -rf',
                                                helperText: 'One pattern per row, such as `rm -rf`, `curl | sh`, or `python -c`.',
                                            })}
                                        </Box>

                                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, gridColumn: { md: '1 / span 2' } }}>
                                            <Stack spacing={1.5}>
                                                {renderCompactListEditor({
                                                    title: 'Limit To Resources',
                                                    description: 'Optional. Add paths only when the command rule should apply to a specific file, directory, URL, or other resource.',
                                                    columnLabel: 'Path / Resource',
                                                    value: editorState.resources,
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
                                                {editorState.group
                                                    ? 'This policy starts from the selected group defaults. Expand to review or override verdict and scope.'
                                                    : 'Configure verdict and scenario scope directly for this standalone policy.'}
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
                                                            {editorState.group
                                                                ? 'Defaults come from the selected group. Change this only when the policy needs a different decision.'
                                                                : 'The verdict defines what Guardrails should do once this policy matches.'}
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
                                                                    label: 'Review',
                                                                    description: 'Mark the result as needing attention without fully blocking it.',
                                                                },
                                                                {
                                                                    value: 'block',
                                                                    label: 'Block',
                                                                    description: 'Stop the content or action and return the policy reason to the user.',
                                                                },
                                                            ].map((option) => {
                                                                const selected = editorState.verdict === option.value;
                                                                return (
                                                                    <Box
                                                                        key={option.value}
                                                                        onClick={() => setEditorState((state) => ({ ...state, verdict: option.value }))}
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
                                                                                {selected && <Chip size="small" color="primary" label="Selected" />}
                                                                            </Stack>
                                                                            <Typography variant="caption" color="text.secondary">
                                                                                {option.description}
                                                                            </Typography>
                                                                        </Stack>
                                                                    </Box>
                                                                );
                                                            })}
                                                        </Box>
                                                    </Box>
                                                </Stack>
                                            </Box>

                                            {renderScenarioScopeSelector({
                                                title: 'Scenario Scope',
                                                description: editorState.group
                                                    ? 'Defaults come from the selected group. Expand and change this only when the policy needs a narrower or broader scope.'
                                                    : 'Choose where this policy applies. By default, standalone policies start enabled for every Guardrails-supported scenario.',
                                                value: editorState.scenarios,
                                                onChange: (scenarios) => setEditorState((state) => ({ ...state, scenarios })),
                                                helperText: editorState.group
                                                    ? 'The current selection was initialized from the chosen group.'
                                                    : 'Leave every scenario selected unless this policy should only apply to a narrower workflow.',
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

            <Dialog
                open={groupDialogOpen}
                onClose={() => {
                    if (!pendingGroupSave) {
                        setGroupDialogOpen(false);
                        blurActiveElement();
                    }
                }}
                disableRestoreFocus
                fullWidth
                maxWidth="sm"
            >
                <DialogTitle>{editingGroupId ? 'Edit group' : 'New group'}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        <Alert severity="info">
                            Groups are used to organize policies and provide shared defaults like severity, default verdict, and scope.
                        </Alert>

                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                            <Stack spacing={2}>
                                <Typography variant="subtitle2">Basic Settings</Typography>
                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                                    <TextField
                                        label="Name"
                                        size="small"
                                        fullWidth
                                        value={groupEditorState.name}
                                        onChange={(e) =>
                                            setGroupEditorState((state) => {
                                                const name = e.target.value;
                                                return {
                                                    ...state,
                                                    name,
                                                    id: editingGroupId ? state.id : generateGroupId(name),
                                                };
                                            })
                                        }
                                        helperText="Human-friendly label shown in UI."
                                    />
                                </Stack>
                                <FormHelperText>
                                    A stable group ID is generated automatically from the name and used by policies internally.
                                </FormHelperText>
                                <FormControlLabel
                                    control={
                                        <Switch
                                            size="small"
                                            checked={groupEditorState.enabled}
                                            onChange={(e) => setGroupEditorState((state) => ({ ...state, enabled: e.target.checked }))}
                                        />
                                    }
                                    label="Enabled"
                                />
                            </Stack>
                        </Box>

                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                            <Stack spacing={2}>
                                <Typography variant="subtitle2">Defaults</Typography>
                                <Typography variant="body2" color="text.secondary">
                                    Defaults are inherited by policies in this group when those policies leave the field empty. They help you define a common baseline once instead of repeating it in every policy.
                                </Typography>
                                <Box>
                                    <Typography variant="caption" color="text.secondary">
                                        Severity
                                    </Typography>
                                    <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" sx={{ mt: 1 }}>
                                        {[
                                            { value: 'low', label: 'Low' },
                                            { value: 'medium', label: 'Medium' },
                                            { value: 'high', label: 'High' },
                                        ].map((option) => (
                                            <Chip
                                                key={option.value}
                                                label={option.label}
                                                clickable
                                                color={groupEditorState.severity === option.value ? 'primary' : 'default'}
                                                variant={groupEditorState.severity === option.value ? 'filled' : 'outlined'}
                                                onClick={() => setGroupEditorState((state) => ({ ...state, severity: option.value }))}
                                            />
                                        ))}
                                    </Stack>
                                    <FormHelperText sx={{ mt: 1 }}>Used for risk grouping and UI labeling.</FormHelperText>
                                </Box>

                                <Box>
                                    <Typography variant="caption" color="text.secondary">
                                        Default Verdict
                                    </Typography>
                                    <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" sx={{ mt: 1 }}>
                                        {[
                                            { value: 'allow', label: 'Allow' },
                                            { value: 'review', label: 'Review' },
                                            { value: 'block', label: 'Block' },
                                        ].map((option) => (
                                            <Chip
                                                key={option.value}
                                                label={option.label}
                                                clickable
                                                color={groupEditorState.defaultVerdict === option.value ? 'primary' : 'default'}
                                                variant={groupEditorState.defaultVerdict === option.value ? 'filled' : 'outlined'}
                                                onClick={() =>
                                                    setGroupEditorState((state) => ({ ...state, defaultVerdict: option.value }))
                                                }
                                            />
                                        ))}
                                    </Stack>
                                    <FormHelperText sx={{ mt: 1 }}>
                                        Used when a policy does not set its own verdict.
                                    </FormHelperText>
                                </Box>
                            </Stack>
                        </Box>

                        {renderScenarioScopeSelector({
                            title: 'Default Scenario Scope',
                            description:
                                'Policies in this group inherit these scenarios unless they explicitly set their own scope.',
                            value: groupEditorState.scenarios,
                            onChange: (scenarios) => setGroupEditorState((state) => ({ ...state, scenarios })),
                            helperText: 'New groups start with every supported scenario selected so their policies apply broadly by default.',
                        })}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button
                        variant="text"
                        onClick={() => {
                            setGroupDialogOpen(false);
                            blurActiveElement();
                        }}
                        disabled={pendingGroupSave}
                    >
                        Cancel
                    </Button>
                    <Button variant="contained" onClick={handleSaveGroup} disabled={pendingGroupSave}>
                        {pendingGroupSave ? 'Saving…' : 'Save'}
                    </Button>
                </DialogActions>
            </Dialog>

            <Dialog open={!!deleteGroupId} onClose={() => setDeleteGroupId(null)} disableRestoreFocus>
                <DialogTitle>Delete group</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary">
                        {deleteGroupId
                            ? `Delete group "${deleteGroupId}"? This only works when no policies still reference the group.`
                            : 'Delete this group?'}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button variant="text" onClick={() => setDeleteGroupId(null)}>
                        Cancel
                    </Button>
                    <Button variant="contained" color="error" onClick={handleDeleteGroup}>
                        Delete
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default GuardrailsRulesPage;
