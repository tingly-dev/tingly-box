import { useEffect, useMemo, useState } from 'react';
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, Tab, Tabs, Typography } from '@mui/material';
import { Rule } from '@/components/icons';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import { useLocation, useNavigate } from 'react-router-dom';
import PolicyEditorDialog, { type PolicyEditorSession } from './rules/PolicyEditorDialog';
import PolicyListSection from './rules/PolicyListSection';
import RegistryCard from './rules/RegistryCard';
import {
    addPendingRegistryInstallId,
    readPendingRegistryInstallIds,
    removePendingRegistryInstallId,
    writePendingRegistryInstallIds,
} from './rules/pendingInstalls';
import {
    applyKindDefaults,
    buildBuiltinPayload,
    ensureDefaultGroupMembership,
    initialRowSelections,
    makeEditorState,
    makeEditorStateFromDraft,
} from './rules/editorState';
import { policyNeedsDisable, policyNeedsEnableWithDefault } from './rules/policyPresentation';
import {
    DEFAULT_GROUP_ID,
    type DisplayPolicy,
    type EditorState,
} from './rules/types';
import { useGuardrailsConfig } from './rules/useGuardrailsConfig';

const GuardrailsRulesPage = () => {
    const notify = useNotify();
    const location = useLocation();
    const navigate = useNavigate();
    const {
        loading,
        loadError,
        groups,
        policies,
        builtins,
        supportedScenarios,
        loadPolicies,
        registryURL,
        registryPolicies,
        registryLoading,
        registryLoadError,
        loadRegistry,
    } = useGuardrailsConfig({ loadBuiltins: true, registry: true, requireScenarios: true });
    const [pendingRegistryInstallIds, setPendingRegistryInstallIds] = useState<Set<string>>(() => readPendingRegistryInstallIds());
    const [pendingPolicyId, setPendingPolicyId] = useState<string | null>(null);
    const [pendingBulkPolicyAction, setPendingBulkPolicyAction] = useState<'enable' | 'disable' | null>(null);
    const [selectedPolicyId, setSelectedPolicyId] = useState<string | null>(null);
    const [deletePolicyId, setDeletePolicyId] = useState<string | null>(null);
    const [selectedPolicyTab, setSelectedPolicyTab] = useState<'resource_access' | 'command_execution' | 'content'>(
        'resource_access'
    );
    const [editorOpen, setEditorOpen] = useState(false);
    const [editorSession, setEditorSession] = useState<PolicyEditorSession | null>(null);

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

    useEffect(() => {
        writePendingRegistryInstallIds(pendingRegistryInstallIds);
    }, [pendingRegistryInstallIds]);

    // ?policyId= / ?ruleId= deep link. Note: unlike the other open paths this
    // never resets the dialog's row selections (session.rows left undefined).
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
        const prepared = makeEditorState(policy, scenarioOptions);
        setEditorSession({
            policyId: policy.id,
            isNewPolicy: false,
            state: prepared.state,
            oversized: prepared.oversized,
        });
        setEditorOpen(true);
        navigate('/guardrails/rules', { replace: true });
    }, [location.search, navigate, policies, scenarioOptions]);

    // newPolicyDraft location-state handoff. No producer exists in the app
    // today (verified 2026-09); kept for the historical cross-page flow.
    useEffect(() => {
        const draft = (location.state as { newPolicyDraft?: Partial<EditorState> } | null)?.newPolicyDraft;
        if (!draft) {
            return;
        }
        if (supportedScenarios.length === 0) {
            return;
        }
        const nextState = makeEditorStateFromDraft(draft, {
            takenIds: policies.map((policy) => policy.id),
            scenarioOptions,
        });
        setEditorSession({
            policyId: null,
            isNewPolicy: true,
            state: nextState,
            oversized: {},
            rows: initialRowSelections(nextState),
        });
        setEditorOpen(true);
        navigate('/guardrails/rules', { replace: true, state: null });
    }, [location.state, navigate, policies, scenarioOptions, supportedScenarios]);

    const openPolicyEditor = (policy: DisplayPolicy) => {
        const builtin = policy.isBuiltin ? builtinMap.get(policy.id) : undefined;
        const isVirtualBuiltin = !!builtin && !policies.some((existing) => existing.id === policy.id);
        const prepared = isVirtualBuiltin
            ? makeEditorState(buildBuiltinPayload(builtin, false), scenarioOptions)
            : makeEditorState(policy, scenarioOptions);
        setEditorSession({
            policyId: isVirtualBuiltin ? null : policy.id,
            isNewPolicy: isVirtualBuiltin,
            state: prepared.state,
            oversized: prepared.oversized,
            rows: initialRowSelections(prepared.state),
        });
        setEditorOpen(true);
    };

    const handleNewPolicy = (kind?: 'resource_access' | 'command_execution' | 'content') => {
        const baseState = makeEditorState(undefined, scenarioOptions).state;
        const nextState = kind
            ? applyKindDefaults(kind, baseState, { isNewPolicy: true, takenIds: policies.map((policy) => policy.id) })
            : baseState;
        setEditorSession({
            policyId: null,
            isNewPolicy: true,
            state: nextState,
            oversized: {},
            rows: initialRowSelections(nextState),
        });
        setEditorOpen(true);
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
                notify.error(result?.error || 'Failed to update policy');
                return;
            }
            await loadPolicies(true);
            notify.success(`Policy "${policyId}" updated.`);
        } catch (error: any) {
            notify.error(error?.message || 'Failed to update policy');
        } finally {
            setPendingPolicyId(null);
        }
    };

    const handleSetPoliciesEnabled = async (enabled: boolean) => {
        try {
            setPendingBulkPolicyAction(enabled ? 'enable' : 'disable');
            const policiesToUpdate = selectedTabPolicies.filter((policy) =>
                enabled ? policyNeedsEnableWithDefault(policy, policies) : policyNeedsDisable(policy, policies)
            );
            if (policiesToUpdate.length === 0) {
                notify.success(
                    enabled
                        ? `All ${selectedTabLabel} policies are already enabled and assigned to Default.`
                        : `All ${selectedTabLabel} policies are already disabled.`
                );
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

            if (failed.length > 0) {
                notify.error(
                    `${enabled ? 'Enabled' : 'Disabled'} ${results.length - failed.length} ${selectedTabLabel} policies. ${failed.length} failed.`
                );
                return;
            }

            notify.success(
                enabled
                    ? `Enabled ${results.length} ${selectedTabLabel} policies and assigned them to Default.`
                    : `Disabled ${results.length} ${selectedTabLabel} policies.`
            );
        } catch (error: any) {
            notify.error(
                error?.message || `Failed to ${enabled ? 'enable' : 'disable'} ${selectedTabLabel.toLowerCase()} policies`
            );
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
                notify.error(result?.error || 'Failed to delete policy');
                return;
            }
            await loadPolicies(true);
            if (selectedPolicyId === deletePolicyId) {
                setSelectedPolicyId(null);
                setEditorOpen(false);
            }
            notify.success(`Policy "${deletePolicyId}" deleted.`);
        } catch (error: any) {
            notify.error(error?.message || 'Failed to delete policy');
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
                notify.error(result?.error || 'Failed to install policy');
                return;
            }
            await loadPolicies(true);
            notify.success(`Policy "${policyId}" installed.`);
        } catch (error: any) {
            notify.error(error?.message || 'Failed to install policy');
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

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Policies"
                    titleHeadingLevel={1}
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
                        {selectedPolicyTab === 'resource_access' && (
                            <PolicyListSection
                                title="Resource Access Policies"
                                description="Use these to control reads, writes, deletes, and other path or resource access behaviors."
                                items={resourceAccessPolicies}
                                kind="resource_access"
                                selectedPolicyId={selectedPolicyId}
                                pendingPolicyId={pendingPolicyId}
                                groupsById={groupsById}
                                onOpen={openPolicyEditor}
                                onToggle={handleTogglePolicy}
                                onDelete={setDeletePolicyId}
                                onNew={handleNewPolicy}
                            />
                        )}
                        {selectedPolicyTab === 'command_execution' && (
                            <PolicyListSection
                                title="Command Execution Policies"
                                description="Use these to control dangerous command execution patterns and shell behavior."
                                items={commandExecutionPolicies}
                                kind="command_execution"
                                selectedPolicyId={selectedPolicyId}
                                pendingPolicyId={pendingPolicyId}
                                groupsById={groupsById}
                                onOpen={openPolicyEditor}
                                onToggle={handleTogglePolicy}
                                onDelete={setDeletePolicyId}
                                onNew={handleNewPolicy}
                            />
                        )}
                        {selectedPolicyTab === 'content' && (
                            <PolicyListSection
                                title="Privacy Policies"
                                description="Use these to filter model output and tool results before they are shown or forwarded."
                                items={contentPolicies}
                                kind="content"
                                selectedPolicyId={selectedPolicyId}
                                pendingPolicyId={pendingPolicyId}
                                groupsById={groupsById}
                                onOpen={openPolicyEditor}
                                onToggle={handleTogglePolicy}
                                onDelete={setDeletePolicyId}
                                onNew={handleNewPolicy}
                            />
                        )}
                    </Stack>
                </UnifiedCard>

                <RegistryCard
                    registryURL={registryURL}
                    registryLoading={registryLoading}
                    registryLoadError={registryLoadError}
                    registryPolicies={registryPolicies}
                    policies={policies}
                    pendingRegistryInstallIds={pendingRegistryInstallIds}
                    onRetry={() => loadRegistry(true)}
                    onInstall={handleInstallRegistryPolicy}
                />
            </Stack>
            <PolicyEditorDialog
                open={editorOpen}
                session={editorSession}
                policies={policies}
                groupOptions={groupOptions}
                scenarioOptions={scenarioOptions}
                selectedPolicyId={selectedPolicyId}
                onSelectedPolicyIdChange={setSelectedPolicyId}
                onClose={() => setEditorOpen(false)}
                onReloadPolicies={() => loadPolicies(true)}
            />
            <Dialog open={!!deletePolicyId} onClose={() => setDeletePolicyId(null)} disableRestoreFocus>
                <DialogTitle>Delete policy</DialogTitle>
                <DialogContent>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
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
