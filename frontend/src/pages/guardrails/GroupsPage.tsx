import { useEffect, useMemo, useState } from 'react';
import {
    Alert,
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    FormHelperText,
    IconButton,
    List,
    ListItem,
    Switch,
    TextField,
    Tooltip,
    Typography,
    Stack,
} from '@mui/material';
import {
    Add,
    AutoAwesome,
    Code as CodeIcon,
    DeleteOutline,
    LaptopMac,
    LockOutlined,
    Rule,
} from '@mui/icons-material';
import { Anthropic, Claude, OpenAI } from '@/components/BrandIcons';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

const DEFAULT_GROUP_ID = 'default';

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
    verdict?: string;
    scope?: {
        scenarios?: string[];
    };
    match?: {
        actions?: { include?: string[] };
        resources?: { values?: string[] };
        terms?: string[];
        patterns?: string[];
        credential_refs?: string[];
    };
};

type GroupEditorState = {
    id: string;
    name: string;
    enabled: boolean;
    severity: string;
    defaultVerdict: string;
    scenarios: string[];
};

const GuardrailsGroupsPage = () => {
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [supportedScenarios, setSupportedScenarios] = useState<string[]>([]);
    const [groups, setGroups] = useState<PolicyGroup[]>([]);
    const [policies, setPolicies] = useState<GuardrailsPolicy[]>([]);
    const [selectedGroupId, setSelectedGroupId] = useState<string>(DEFAULT_GROUP_ID);
    const [groupDialogOpen, setGroupDialogOpen] = useState(false);
    const [editingGroupId, setEditingGroupId] = useState<string | null>(null);
    const [deleteGroupId, setDeleteGroupId] = useState<string | null>(null);
    const [pendingGroupId, setPendingGroupId] = useState<string | null>(null);
    const [pendingGroupSave, setPendingGroupSave] = useState(false);
    const [initializingDefaultGroup, setInitializingDefaultGroup] = useState(false);
    const [groupEditorState, setGroupEditorState] = useState<GroupEditorState>({
        id: '',
        name: '',
        enabled: true,
        severity: 'medium',
        defaultVerdict: 'block',
        scenarios: [],
    });

    const scenarioOptions = useMemo(() => supportedScenarios.filter(Boolean), [supportedScenarios]);
    const groupsById = useMemo(() => new Map(groups.map((group) => [group.id, group])), [groups]);

    const sortedGroups = useMemo(() => {
        const next = [...groups];
        next.sort((a, b) => {
            if (a.id === DEFAULT_GROUP_ID) return -1;
            if (b.id === DEFAULT_GROUP_ID) return 1;
            return (a.name || a.id).localeCompare(b.name || b.id);
        });
        return next;
    }, [groups]);

    const effectivePolicyGroup = (policy: GuardrailsPolicy) => policy.group?.trim() || DEFAULT_GROUP_ID;

    const selectedGroup = useMemo(
        () => groups.find((group) => group.id === selectedGroupId) || groupsById.get(DEFAULT_GROUP_ID),
        [groups, groupsById, selectedGroupId]
    );

    const groupPolicyCount = (groupId: string) => policies.filter((policy) => policy.enabled !== false && effectivePolicyGroup(policy) === groupId).length;

    const buildGroupSummary = (group: PolicyGroup) => {
        const severity = group.severity || 'medium';
        const verdict = group.default_verdict || 'block';
        const scenarios = group.default_scope?.scenarios?.join(', ') || 'all scenarios';
        return `${severity} · ${verdict} · ${scenarios}`;
    };

    const buildPolicySummary = (policy: GuardrailsPolicy) => {
        if (policy.kind === 'command_execution') {
            const terms = policy.match?.terms?.join(', ') || 'any command';
            return terms;
        }
        if (policy.kind === 'resource_access' || policy.kind === 'operation') {
            const actions = policy.match?.actions?.include?.join(', ') || 'any action';
            const resources = policy.match?.resources?.values?.join(', ') || 'any resource';
            return `${actions} · ${resources}`;
        }
        const patterns = policy.match?.patterns || [];
        return patterns.slice(0, 2).join(', ') || 'No patterns configured';
    };

    const buildPolicyKindLabel = (policy: GuardrailsPolicy) => {
        if (policy.kind === 'resource_access' || policy.kind === 'operation') return 'Resource Access';
        if (policy.kind === 'command_execution') return 'Command Execution';
        return 'Privacy';
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

    const toggleScenario = (values: string[], value: string) => {
        if (values.includes(value)) {
            return values.filter((item) => item !== value);
        }
        return [...values, value];
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
        onChange: (nextValue: string[]) => void;
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
                                onClick={() => onChange(toggleScenario(value, option))}
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

    const loadConfig = async (silent = false) => {
        try {
            if (!silent) setLoading(true);
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
            if (!silent) setLoading(false);
        }
    };

    useEffect(() => {
        loadConfig();
    }, []);

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
                    default_verdict: 'block',
                    default_scope: { scenarios: supportedScenarios },
                });
                if (!result?.success) {
                    setActionMessage({ type: 'error', text: result?.error || 'Failed to create default group.' });
                    return;
                }
                await loadConfig(true);
            } catch (error: any) {
                setActionMessage({ type: 'error', text: error?.message || 'Failed to create default group.' });
            } finally {
                setInitializingDefaultGroup(false);
            }
        };

        ensureDefaultGroup();
    }, [groups, initializingDefaultGroup, loadError, loading, supportedScenarios]);

    useEffect(() => {
        if (groups.length === 0) return;
        if (!groups.some((group) => group.id === selectedGroupId)) {
            setSelectedGroupId(DEFAULT_GROUP_ID);
        }
    }, [groups, selectedGroupId]);

    const openNewGroupDialog = () => {
        setEditingGroupId(null);
        setGroupEditorState(makeGroupEditorState());
        setGroupDialogOpen(true);
    };

    const openEditGroupDialog = (group: PolicyGroup) => {
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
                setActionMessage({ type: 'error', text: result?.error || 'Failed to save group.' });
                return;
            }
            await loadConfig(true);
            setSelectedGroupId(payload.id);
            setGroupDialogOpen(false);
            setActionMessage({ type: 'success', text: `Group "${groupEditorState.id}" saved.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to save group.' });
        } finally {
            setPendingGroupSave(false);
        }
    };

    const handleDeleteGroup = async () => {
        if (!deleteGroupId || deleteGroupId === DEFAULT_GROUP_ID) {
            return;
        }
        try {
            setPendingGroupId(deleteGroupId);
            const result = await api.deleteGuardrailsGroup(deleteGroupId);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to delete group.' });
                return;
            }
            await loadConfig(true);
            setDeleteGroupId(null);
            setActionMessage({ type: 'success', text: `Group "${deleteGroupId}" deleted.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to delete group.' });
        } finally {
            setPendingGroupId(null);
        }
    };

    const handleToggleGroup = async (groupId: string, enabled: boolean) => {
        const group = groups.find((item) => item.id === groupId);
        if (!group) {
            return;
        }

        try {
            setPendingGroupId(groupId);
            const result = await api.updateGuardrailsGroup(groupId, {
                id: group.id,
                name: group.name || group.id,
                enabled,
                severity: group.severity || 'medium',
                default_verdict: group.default_verdict || 'block',
                default_scope: {
                    scenarios: group.default_scope?.scenarios || [],
                },
            });
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update group.' });
                return;
            }
            await loadConfig(true);
            setActionMessage({ type: 'success', text: `Group "${groupId}" updated.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to update group.' });
        } finally {
            setPendingGroupId(null);
        }
    };

    const handleAssignPolicy = async (policy: GuardrailsPolicy, checked: boolean) => {
        if (!selectedGroup) {
            return;
        }
        if (selectedGroup.id === DEFAULT_GROUP_ID && !checked) {
            setActionMessage({
                type: 'error',
                text: 'Policies must belong to a group. To move a policy out of Default, assign it from another group.',
            });
            return;
        }

        try {
            const result = await api.updateGuardrailsPolicy(policy.id, {
                group: checked ? selectedGroup.id : DEFAULT_GROUP_ID,
            });
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update policy group.' });
                return;
            }
            await loadConfig(true);
            setActionMessage({
                type: 'success',
                text:
                    selectedGroup.id === DEFAULT_GROUP_ID
                        ? `Policy "${policy.id}" moved to Default.`
                        : checked
                          ? `Policy "${policy.id}" added to ${selectedGroup.name || selectedGroup.id}.`
                          : `Policy "${policy.id}" moved back to Default.`,
            });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to update policy group.' });
        }
    };

    const policiesInSelectedGroup = useMemo(
        () => policies.filter((policy) => policy.enabled !== false && effectivePolicyGroup(policy) === (selectedGroup?.id || DEFAULT_GROUP_ID)),
        [policies, selectedGroup]
    );

    const visiblePolicies = useMemo(
        () => policies.filter((policy) => policy.enabled !== false),
        [policies]
    );

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Policy Groups"
                    subtitle="Use groups to manage shared defaults and assign policies without editing each rule individually."
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button variant="contained" size="small" startIcon={<Add />} onClick={openNewGroupDialog}>
                                New Group
                            </Button>
                        </Stack>
                    }
                >
                    <Stack spacing={1.5}>
                        {loadError && <Alert severity="error">{loadError}</Alert>}
                        {actionMessage && <Alert severity={actionMessage.type}>{actionMessage.text}</Alert>}
                        <Typography variant="body2" color="text.secondary">
                            The Default group is always present. New policies start there unless you explicitly assign them to another group.
                        </Typography>
                    </Stack>
                </UnifiedCard>

                <UnifiedCard title={`Groups (${groups.length})`} size="full">
                    <List dense sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, py: 0, overflow: 'hidden' }}>
                        {sortedGroups.map((group) => {
                            const selected = selectedGroupId === group.id;
                            const isDefaultGroup = group.id === DEFAULT_GROUP_ID;
                            return (
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
                                            bgcolor: selected ? 'action.selected' : 'transparent',
                                            '&:hover': { bgcolor: 'action.hover' },
                                        }}
                                        onClick={() => setSelectedGroupId(group.id)}
                                    >
                                        <Box sx={{ minWidth: { md: 220 }, flexShrink: 0 }}>
                                            <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                    {group.name || group.id}
                                                </Typography>
                                                <Chip size="small" label={group.id} variant="outlined" />
                                                {isDefaultGroup && <Chip size="small" label="Default" icon={<LockOutlined />} />}
                                                {selected && <Chip size="small" color="primary" label="Selected" />}
                                            </Stack>
                                        </Box>

                                        <Box sx={{ flex: 1, minWidth: 0 }}>
                                            <Typography variant="body2" color="text.primary" sx={{ whiteSpace: 'normal' }}>
                                                {buildGroupSummary(group)}
                                            </Typography>
                                            <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
                                                {groupPolicyCount(group.id)} polic{groupPolicyCount(group.id) === 1 ? 'y' : 'ies'} assigned
                                            </Typography>
                                        </Box>

                                        <Stack direction="row" spacing={1} alignItems="center" sx={{ width: { xs: '100%', md: 'auto' }, justifyContent: { xs: 'space-between', md: 'flex-end' } }}>
                                            <Chip size="small" label={group.enabled === false ? 'Disabled' : 'Enabled'} />
                                            <Switch
                                                size="small"
                                                checked={group.enabled !== false}
                                                disabled={pendingGroupId === group.id}
                                                onClick={(e) => e.stopPropagation()}
                                                onChange={(e) => handleToggleGroup(group.id, e.target.checked)}
                                            />
                                            <Tooltip title="Edit group" arrow>
                                                <span>
                                                    <Button
                                                        size="small"
                                                        variant="text"
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            openEditGroupDialog(group);
                                                        }}
                                                    >
                                                        Edit
                                                    </Button>
                                                </span>
                                            </Tooltip>
                                            {!isDefaultGroup && (
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
                                            )}
                                        </Stack>
                                    </Box>
                                </ListItem>
                            );
                        })}
                    </List>
                </UnifiedCard>

                <UnifiedCard
                    title={selectedGroup?.id === DEFAULT_GROUP_ID ? 'Policies in Default' : `Assign Policies · ${selectedGroup?.name || selectedGroup?.id || ''}`}
                    subtitle={
                        selectedGroup?.id === DEFAULT_GROUP_ID
                            ? 'Turn a policy on to move it into Default. To move a policy out of Default, assign it from another group.'
                            : 'Check a policy to assign it to this group. Unchecking moves it back to Default.'
                    }
                    size="full"
                >
                    <List dense sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, py: 0, overflow: 'hidden' }}>
                        {visiblePolicies.length === 0 ? (
                            <ListItem>
                                <Typography variant="body2" color="text.secondary">
                                    No enabled policies are available.
                                </Typography>
                            </ListItem>
                        ) : (
                            visiblePolicies.map((policy) => {
                                const checked = effectivePolicyGroup(policy) === selectedGroup?.id;
                                return (
                                    <ListItem
                                        key={policy.id}
                                        sx={{ px: 2, py: 1.5, borderBottom: '1px solid', borderColor: 'divider', '&:last-child': { borderBottom: 'none' } }}
                                    >
                                        <Stack direction="row" spacing={1.5} alignItems="center" sx={{ width: '100%' }}>
                                            <Switch
                                                size="small"
                                                checked={checked}
                                                onChange={(e) => handleAssignPolicy(policy, e.target.checked)}
                                            />
                                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                                <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                                    <Typography variant="body2" sx={{ fontWeight: 600 }}>{policy.name || policy.id}</Typography>
                                                    <Chip size="small" label={buildPolicyKindLabel(policy)} variant="outlined" />
                                                    <Chip size="small" label={`Current: ${groupsById.get(effectivePolicyGroup(policy))?.name || effectivePolicyGroup(policy)}`} variant="outlined" />
                                                </Stack>
                                                <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
                                                    {buildPolicySummary(policy)}
                                                </Typography>
                                            </Box>
                                        </Stack>
                                    </ListItem>
                                );
                            })
                        )}
                    </List>
                </UnifiedCard>
            </Stack>

            <Dialog
                open={groupDialogOpen}
                onClose={() => {
                    if (!pendingGroupSave) {
                        setGroupDialogOpen(false);
                    }
                }}
                fullWidth
                maxWidth="sm"
                disableRestoreFocus
            >
                <DialogTitle>{editingGroupId ? 'Edit Group' : 'New Group'}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ pt: 1 }}>
                        {actionMessage && <Alert severity={actionMessage.type}>{actionMessage.text}</Alert>}
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
                                            { value: 'review', label: 'Ask', disabled: true },
                                            { value: 'block', label: 'Block' },
                                        ].map((option) => (
                                            <Tooltip key={option.value} title={option.disabled ? 'Reserved for a future interactive verdict.' : ''} disableHoverListener={!option.disabled}>
                                                <span>
                                                    <Chip
                                                        label={option.label}
                                                        clickable={!option.disabled}
                                                        disabled={option.disabled}
                                                        color={groupEditorState.defaultVerdict === option.value ? 'primary' : 'default'}
                                                        variant={groupEditorState.defaultVerdict === option.value ? 'filled' : 'outlined'}
                                                        onClick={() => {
                                                            if (option.disabled) return;
                                                            setGroupEditorState((state) => ({ ...state, defaultVerdict: option.value }));
                                                        }}
                                                    />
                                                </span>
                                            </Tooltip>
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
                            helperText:
                                'New groups start with every supported scenario selected so their policies apply broadly by default.',
                        })}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setGroupDialogOpen(false)} disabled={pendingGroupSave}>Cancel</Button>
                    <Button variant="contained" onClick={handleSaveGroup} disabled={pendingGroupSave}>
                        {pendingGroupSave ? 'Saving…' : 'Save'}
                    </Button>
                </DialogActions>
            </Dialog>

            <Dialog open={!!deleteGroupId} onClose={() => setDeleteGroupId(null)} disableRestoreFocus>
                <DialogTitle>Delete Group</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" color="text.secondary">
                        {deleteGroupId
                            ? `Delete group "${deleteGroupId}"? This only works when no policies still reference it.`
                            : 'Delete this group?'}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setDeleteGroupId(null)}>Cancel</Button>
                    <Button variant="contained" color="error" onClick={handleDeleteGroup}>Delete</Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default GuardrailsGroupsPage;
