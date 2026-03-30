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
    DeleteOutline,
    LockOutlined,
} from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

const DEFAULT_GROUP_ID = 'default';

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
};

const GuardrailsGroupsPage = () => {
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
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
    });
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

    const effectivePolicyGroups = (policy: GuardrailsPolicy) => {
        const values = Array.isArray(policy.groups) ? policy.groups : [];
        return Array.from(new Set(values));
    };

    const selectedGroup = useMemo(
        () => groups.find((group) => group.id === selectedGroupId) || groupsById.get(DEFAULT_GROUP_ID),
        [groups, groupsById, selectedGroupId]
    );

    const groupPolicyCount = (groupId: string) => policies.filter((policy) => policy.enabled !== false && effectivePolicyGroups(policy).includes(groupId)).length;

    const buildGroupSummary = (group: PolicyGroup) => `${group.severity || 'medium'} severity`;

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

    const loadConfig = async (silent = false) => {
        try {
            if (!silent) setLoading(true);
            const guardrailsConfig = await api.getGuardrailsConfig();
            const config = guardrailsConfig?.config || {};
            setGroups(Array.isArray(config.groups) ? config.groups : []);
            setPolicies(Array.isArray(config.policies) ? config.policies : []);
            setLoadError(null);
        } catch (error) {
            console.error('Failed to load guardrails config:', error);
            setGroups([]);
            setPolicies([]);
            setLoadError('Failed to load guardrails config');
        } finally {
            if (!silent) setLoading(false);
        }
    };

    useEffect(() => {
        loadConfig();
    }, []);

    useEffect(() => {
        if (loading || loadError || initializingDefaultGroup) {
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
    }, [groups, initializingDefaultGroup, loadError, loading]);

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

        const nextGroups = checked
            ? Array.from(new Set([...effectivePolicyGroups(policy), selectedGroup.id]))
            : effectivePolicyGroups(policy).filter((groupID) => groupID !== selectedGroup.id);

        try {
            const result = await api.updateGuardrailsPolicy(policy.id, {
                groups: nextGroups,
            });
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update policy group.' });
                return;
            }
            await loadConfig(true);
            setActionMessage({
                type: 'success',
                text: checked
                    ? `Policy "${policy.id}" added to ${selectedGroup.name || selectedGroup.id}.`
                    : `Policy "${policy.id}" removed from ${selectedGroup.name || selectedGroup.id}.`,
            });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to update policy group.' });
        }
    };

    const visiblePolicies = useMemo(
        () => policies.filter((policy) => policy.enabled !== false),
        [policies]
    );

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Policy Groups"
                    subtitle="Groups organize policies and control whether those policy sets participate in evaluation. Built-in is a policy label, not a group type."
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
                            Groups are collections. A policy can appear in multiple groups, and a group only contributes activation when it is enabled.
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
                                        <Box sx={{ minWidth: { md: 180 }, flexShrink: 0 }}>
                                            <Stack direction="row" spacing={1} alignItems="center" useFlexGap flexWrap="wrap">
                                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                    {group.name || group.id}
                                                </Typography>
                                                {isDefaultGroup && <LockOutlined sx={{ fontSize: 16, color: 'text.secondary' }} />}
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

                                        <Stack
                                            direction="row"
                                            spacing={1}
                                            alignItems="center"
                                            sx={{
                                                width: { xs: '100%', md: 220 },
                                                minWidth: { md: 220 },
                                                justifyContent: { xs: 'space-between', md: 'flex-end' },
                                                flexShrink: 0,
                                            }}
                                        >
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
                                            <Box sx={{ width: 32, display: 'flex', justifyContent: 'center', flexShrink: 0 }}>
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
                                            </Box>
                                        </Stack>
                                    </Box>
                                </ListItem>
                            );
                        })}
                    </List>
                </UnifiedCard>

                <UnifiedCard
                    title={selectedGroup?.id === DEFAULT_GROUP_ID ? 'Policies in Default' : `Assign Policies · ${selectedGroup?.name || selectedGroup?.id || ''}`}
                    subtitle="Check a policy to include it in this group. Only enabled policies appear here."
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
                                const checked = !!selectedGroup && effectivePolicyGroups(policy).includes(selectedGroup.id);
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
                                                </Stack>
                                                <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
                                                    <Box
                                                        component="span"
                                                        sx={{
                                                            display: 'block',
                                                            whiteSpace: 'nowrap',
                                                            overflow: 'hidden',
                                                            textOverflow: 'ellipsis',
                                                        }}
                                                    >
                                                        {buildPolicySummary(policy)}
                                                    </Box>
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
                            </Stack>
                        </Box>

                        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                            <Stack spacing={2}>
                                <Typography variant="subtitle2">Group State</Typography>

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
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <Switch
                                        size="small"
                                        checked={groupEditorState.enabled}
                                        onChange={(e) => setGroupEditorState((state) => ({ ...state, enabled: e.target.checked }))}
                                    />
                                    <Typography variant="body2">Enabled</Typography>
                                </Stack>
                            </Stack>
                        </Box>
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
