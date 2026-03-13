import { useEffect, useMemo, useState } from 'react';
import {
    Box,
    Stack,
    Typography,
    Chip,
    Button,
    Divider,
    Alert,
    List,
    ListItem,
    ListItemText,
    Accordion,
    AccordionSummary,
    AccordionDetails,
    Collapse,
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Tooltip,
    TextField,
    FormControl,
    InputLabel,
    FormHelperText,
    Select,
    MenuItem,
    Switch,
    FormControlLabel,
    Checkbox,
    FormGroup,
    IconButton,
} from '@mui/material';
import { Rule, ExpandMore, Refresh as RefreshIcon, DeleteOutline } from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { useLocation, useNavigate } from 'react-router-dom';

type GuardrailsRule = {
    id: string;
    type: string;
    scope: string;
    status: string;
    reason: string;
    enabled: boolean;
};

const defaultScenarioOptions = ['anthropic', 'claude_code', 'openai'];
const defaultDirectionOptions = ['request', 'response'];
const defaultContentTypeOptions = ['command', 'text', 'messages'];
const defaultCommandKindOptions = ['shell'];
const defaultCommandActionOptions = ['read', 'write', 'delete', 'execute', 'transfer', 'redirect'];

const GuardrailsRulesPage = () => {
    const location = useLocation();
    const navigate = useNavigate();
    const [loading, setLoading] = useState(true);
    const [rules, setRules] = useState<GuardrailsRule[]>([]);
    const [rawRules, setRawRules] = useState<any[]>([]);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [pendingRuleId, setPendingRuleId] = useState<string | null>(null);
    const [pendingSave, setPendingSave] = useState(false);
    const [selectedRuleId, setSelectedRuleId] = useState<string | null>(null);
    const [isNewRule, setIsNewRule] = useState(false);
    const [editorOpen, setEditorOpen] = useState(false);
    const [editorSnapshot, setEditorSnapshot] = useState('');
    const [confirmCloseOpen, setConfirmCloseOpen] = useState(false);
    const [deleteRuleId, setDeleteRuleId] = useState<string | null>(null);
    const [editorState, setEditorState] = useState({
        id: '',
        name: '',
        type: 'text_match',
        verdict: 'block',
        enabled: true,
        scenarios: defaultScenarioOptions,
        directions: defaultDirectionOptions,
        contentTypes: defaultContentTypeOptions,
        patterns: '',
        reason: '',
        targets: ['command'],
        kinds: defaultCommandKindOptions,
        actions: ['read'],
        resources: '',
        resourceMatch: 'prefix',
        terms: '',
    });

    const isEditorDirty = useMemo(() => {
        if (!editorSnapshot) {
            return false;
        }
        return JSON.stringify(editorState) !== editorSnapshot;
    }, [editorState, editorSnapshot]);

    const scenarioOptions = useMemo(() => {
        const fromRules = rawRules.flatMap((rule) => rule?.scope?.scenarios ?? []);
        return Array.from(new Set([...defaultScenarioOptions, ...fromRules])).filter(Boolean);
    }, [rawRules]);

    const directionOptions = useMemo(() => {
        const fromRules = rawRules.flatMap((rule) => rule?.scope?.directions ?? []);
        return Array.from(new Set([...defaultDirectionOptions, ...fromRules])).filter(Boolean);
    }, [rawRules]);

    const contentTypeOptions = useMemo(() => {
        const fromRules = rawRules.flatMap((rule) => rule?.scope?.content_types ?? []);
        return Array.from(new Set([...defaultContentTypeOptions, ...fromRules])).filter(Boolean);
    }, [rawRules]);

    const targetOptions = contentTypeOptions;
    const commandActionOptions = defaultCommandActionOptions;
    const commandKindOptions = defaultCommandKindOptions;

    const toggleValue = (values: string[], value: string) => {
        if (values.includes(value)) {
            return values.filter((item) => item !== value);
        }
        return [...values, value];
    };

    const splitLines = (value: string) =>
        value
            .split('\n')
            .map((item) => item.trim())
            .filter(Boolean);

    const buildSuggestedReason = (state: typeof editorState) => {
        if (state.type === 'command_policy') {
            const actionPhrases: Record<string, string> = {
                read: 'read',
                write: 'modify',
                delete: 'delete',
                execute: 'execute',
                transfer: 'transfer',
                redirect: 'redirect output from',
            };
            const actions = state.actions.length > 0
                ? state.actions.map((action) => actionPhrases[action] ?? action).join(', ')
                : 'access';
            const resources = splitLines(state.resources);
            const resourceLabel = resources.length > 0 ? resources.join(', ') : 'protected resources';
            return `This rule blocks attempts to ${actions} ${resourceLabel}.`;
        }
        if (state.type === 'text_match') {
            const patterns = splitLines(state.patterns);
            if (patterns.length === 0) {
                return 'This rule blocks prohibited content.';
            }
            return `This rule blocks content matching ${patterns.slice(0, 2).join(', ')}.`;
        }
        return 'This guardrails rule was triggered.';
    };

    useEffect(() => {
        const loadFlags = async () => {
            try {
                setLoading(true);
                const guardrailsConfig = await api.getGuardrailsConfig();
                if (guardrailsConfig?.config?.rules) {
                    setRules(mapRules(guardrailsConfig.config.rules));
                    setRawRules(guardrailsConfig.config.rules);
                } else {
                    setRules([]);
                    setRawRules([]);
                }
                setLoadError(null);
            } catch (error) {
                console.error('Failed to load guardrails flags:', error);
                setRules([]);
                setRawRules([]);
                setLoadError('Failed to load guardrails config');
            } finally {
                setLoading(false);
            }
        };
        loadFlags();
    }, []);

    useEffect(() => {
        const params = new URLSearchParams(location.search);
        const ruleId = params.get('ruleId');
        if (!ruleId || rawRules.length === 0) {
            return;
        }
        const targetRule = rules.find((rule) => rule.id === ruleId);
        if (!targetRule) {
            return;
        }
        setSelectedRuleId(ruleId);
        updateEditorFromRule(targetRule);
        navigate('/guardrails/rules', { replace: true });
    }, [location.search, rawRules, rules, navigate]);

    const mapRules = (rawRules: any[]): GuardrailsRule[] => {
        return rawRules.map((rule: any) => {
            const scope = rule.scope || {};
            const params = rule.params || {};
            const contentTypes = Array.isArray(scope.content_types) ? scope.content_types.join(', ') : 'all';
            const directions = Array.isArray(scope.directions) ? scope.directions.join(', ') : 'all';
            const scenarios = Array.isArray(scope.scenarios) ? scope.scenarios.join(', ') : 'all';
            const scopeText = `${contentTypes} · ${directions} · ${scenarios}`;
            const semanticSummary =
                rule.type === 'command_policy'
                    ? buildCommandPolicySummary(params)
                    : (params.reason || 'n/a');
            return {
                id: rule.id || 'unknown',
                type: rule.type || 'unknown',
                scope: scopeText,
                status: rule.enabled ? 'Enabled' : 'Disabled',
                reason: semanticSummary,
                enabled: !!rule.enabled,
            };
        });
    };

    const buildCommandPolicySummary = (params: any) => {
        const actions = Array.isArray(params.actions) ? params.actions.join(', ') : 'any action';
        const resources = Array.isArray(params.resources) && params.resources.length > 0
            ? params.resources.join(', ')
            : 'any resource';
        const matchMode = params.resource_match ? ` (${params.resource_match})` : '';
        return `${actions} -> ${resources}${matchMode}`;
    };

    const updateEditorFromRule = (rule: GuardrailsRule) => {
        const rawRule = rawRules.find((r) => r.id === rule.id) || {};
        const scope = rawRule.scope || {};
        const params = rawRule.params || {};
        const patterns = Array.isArray(params.patterns) ? params.patterns.join('\n') : '';
        const resources = Array.isArray(params.resources) ? params.resources.join('\n') : '';
        const terms = Array.isArray(params.terms) ? params.terms.join('\n') : '';
        const nextState = {
            id: rule.id,
            name: rawRule.name || rule.id,
            type: rawRule.type || rule.type || 'text_match',
            verdict: params.verdict || 'block',
            enabled: rule.enabled,
            scenarios: Array.isArray(scope.scenarios) ? scope.scenarios : [],
            directions: Array.isArray(scope.directions) ? scope.directions : [],
            contentTypes: Array.isArray(scope.content_types) ? scope.content_types : [],
            targets: Array.isArray(params.targets) ? params.targets : [],
            patterns,
            reason: params.reason || rule.reason || '',
            kinds: Array.isArray(params.kinds) ? params.kinds : ['shell'],
            actions: Array.isArray(params.actions) ? params.actions : [],
            resources,
            resourceMatch: params.resource_match || 'prefix',
            terms,
        };
        setEditorState(nextState);
        setIsNewRule(false);
        setEditorOpen(true);
        setEditorSnapshot(JSON.stringify(nextState));
    };

    const handleToggleRule = async (ruleId: string, enabled: boolean) => {
        try {
            setPendingRuleId(ruleId);
            const result = await api.updateGuardrailsRule(ruleId, { enabled });
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update rule' });
                return;
            }
            const guardrailsConfig = await api.getGuardrailsConfig();
            const nextRules = guardrailsConfig?.config?.rules || [];
            setRules(nextRules.length ? mapRules(nextRules) : []);
            setRawRules(nextRules);
            if (selectedRuleId === ruleId) {
                const updated = nextRules.find((r: any) => r.id === ruleId);
                if (updated) {
                    setEditorState((state) => ({
                        ...state,
                        enabled: !!updated.enabled,
                    }));
                    setEditorSnapshot((snapshot) => {
                        if (!snapshot) {
                            return snapshot;
                        }
                        const nextSnapshot = JSON.parse(snapshot);
                        nextSnapshot.enabled = !!updated.enabled;
                        return JSON.stringify(nextSnapshot);
                    });
                }
            }
            setActionMessage({ type: 'success', text: `Rule "${ruleId}" updated.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to update rule' });
        } finally {
            setPendingRuleId(null);
        }
    };

    const handleSaveRule = async (): Promise<boolean> => {
        if (!editorState.id) {
            setActionMessage({ type: 'error', text: 'Rule ID is required.' });
            return false;
        }
        try {
            setPendingSave(true);
            const basePayload = {
                id: editorState.id,
                name: editorState.name,
                type: editorState.type,
                enabled: editorState.enabled,
                scope: {
                    scenarios: editorState.scenarios,
                    directions: editorState.directions,
                    content_types: editorState.contentTypes,
                },
            };
            const params =
                editorState.type === 'command_policy'
                    ? {
                          kinds: editorState.kinds,
                          actions: editorState.actions,
                          resources: splitLines(editorState.resources),
                          resource_match: editorState.resourceMatch,
                          terms: splitLines(editorState.terms),
                          verdict: editorState.verdict,
                          reason: editorState.reason,
                      }
                    : {
                          patterns: splitLines(editorState.patterns),
                          verdict: editorState.verdict,
                          reason: editorState.reason,
                          targets: editorState.targets,
                      };
            const payload = {
                ...basePayload,
                params,
            };
            const result = isNewRule
                ? await api.createGuardrailsRule(payload)
                : await api.updateGuardrailsRule(editorState.id, payload);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to save rule' });
                return false;
            }
            const guardrailsConfig = await api.getGuardrailsConfig();
            setRules(guardrailsConfig?.config?.rules ? mapRules(guardrailsConfig.config.rules) : []);
            setRawRules(guardrailsConfig?.config?.rules || []);
            setActionMessage({ type: 'success', text: `Rule "${editorState.id}" saved.` });
            setIsNewRule(false);
            setEditorSnapshot(JSON.stringify(editorState));
            return true;
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to save rule' });
            return false;
        } finally {
            setPendingSave(false);
        }
    };

    const handleNewRule = () => {
        setSelectedRuleId(null);
        setIsNewRule(true);
        const nextState = {
            id: 'new-rule',
            name: 'New Rule',
            type: 'command_policy',
            verdict: 'block',
            enabled: true,
            scenarios: scenarioOptions,
            directions: directionOptions,
            contentTypes: contentTypeOptions,
            patterns: '',
            reason: '',
            targets: ['command'],
            kinds: commandKindOptions,
            actions: ['read'],
            resources: '',
            resourceMatch: 'prefix',
            terms: '',
        };
        setEditorState(nextState);
        setEditorOpen(true);
        setEditorSnapshot(JSON.stringify(nextState));
    };

    const handleDuplicateRule = async () => {
        if (!editorState.id) {
            setActionMessage({ type: 'error', text: 'No rule selected to duplicate.' });
            return;
        }
        const existingIds = new Set(rules.map((rule) => rule.id));
        const baseId = `${editorState.id}-copy`;
        let newId = baseId;
        let suffix = 2;
        while (existingIds.has(newId)) {
            newId = `${baseId}-${suffix}`;
            suffix += 1;
        }
        try {
            setPendingSave(true);
            const params =
                editorState.type === 'command_policy'
                    ? {
                          kinds: editorState.kinds,
                          actions: editorState.actions,
                          resources: splitLines(editorState.resources),
                          resource_match: editorState.resourceMatch,
                          terms: splitLines(editorState.terms),
                          verdict: editorState.verdict,
                          reason: editorState.reason,
                      }
                    : {
                          patterns: splitLines(editorState.patterns),
                          verdict: editorState.verdict,
                          reason: editorState.reason,
                          targets: editorState.targets,
                      };
            const payload = {
                id: newId,
                name: `${editorState.name} (copy)`,
                type: editorState.type,
                enabled: editorState.enabled,
                scope: {
                    scenarios: editorState.scenarios,
                    directions: editorState.directions,
                    content_types: editorState.contentTypes,
                },
                params,
            };
            const result = await api.createGuardrailsRule(payload);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to duplicate rule' });
                return;
            }
            const guardrailsConfig = await api.getGuardrailsConfig();
            setRules(guardrailsConfig?.config?.rules ? mapRules(guardrailsConfig.config.rules) : []);
            setRawRules(guardrailsConfig?.config?.rules || []);
            setSelectedRuleId(newId);
            setEditorState((state) => {
                const nextState = {
                    ...state,
                    id: newId,
                    name: `${state.name} (copy)`,
                };
                setEditorSnapshot(JSON.stringify(nextState));
                return nextState;
            });
            setIsNewRule(false);
            setActionMessage({ type: 'success', text: `Rule "${newId}" created.` });
            setEditorOpen(true);
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to duplicate rule' });
        } finally {
            setPendingSave(false);
        }
    };

    const handleReload = async () => {
        try {
            setLoading(true);
            const result = await api.reloadGuardrailsConfig();
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to reload guardrails config' });
                return;
            }
            const guardrailsConfig = await api.getGuardrailsConfig();
            setRules(guardrailsConfig?.config?.rules ? mapRules(guardrailsConfig.config.rules) : []);
            setActionMessage({ type: 'success', text: 'Guardrails reloaded successfully.' });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to reload guardrails config' });
        } finally {
            setLoading(false);
        }
    };

    const actionAlert = useMemo(() => {
        if (!actionMessage) {
            return null;
        }
        return <Alert severity={actionMessage.type}>{actionMessage.text}</Alert>;
    }, [actionMessage]);

    const handleCloseEditor = async () => {
        if (isEditorDirty) {
            setConfirmCloseOpen(true);
            return;
        }
        setEditorOpen(false);
    };

    const handleConfirmClose = async (action: 'save' | 'discard' | 'cancel') => {
        if (action === 'cancel') {
            setConfirmCloseOpen(false);
            return;
        }
        if (action === 'save') {
            const saved = await handleSaveRule();
            if (!saved) {
                return;
            }
        }
        setConfirmCloseOpen(false);
        setEditorOpen(false);
    };

    const handleDeleteRule = async () => {
        if (!deleteRuleId) {
            return;
        }
        try {
            setPendingRuleId(deleteRuleId);
            const result = await api.deleteGuardrailsRule(deleteRuleId);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to delete rule' });
                return;
            }
            const guardrailsConfig = await api.getGuardrailsConfig();
            const nextRules = guardrailsConfig?.config?.rules || [];
            setRules(nextRules.length ? mapRules(nextRules) : []);
            setRawRules(nextRules);
            if (selectedRuleId === deleteRuleId) {
                setSelectedRuleId(null);
                setEditorOpen(false);
            }
            setActionMessage({ type: 'success', text: `Rule "${deleteRuleId}" deleted.` });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to delete rule' });
        } finally {
            setPendingRuleId(null);
            setDeleteRuleId(null);
        }
    };

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Guardrails Rules"
                    subtitle="Create and maintain the rules used to block risky tool calls and filter sensitive tool results."
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
                            Configure pre-execution blocks (tool_use) and post-execution filters (tool_result).
                        </Typography>
                        <Divider />
                    </Stack>
                </UnifiedCard>

                <Box sx={{ width: '100%' }}>
                    <UnifiedCard
                        title="Rules"
                        subtitle={`${rules.length} rule${rules.length === 1 ? '' : 's'} configured`}
                        size="full"
                        rightAction={
                            <Stack direction="row" spacing={1}>
                                <Button variant="contained" size="small" startIcon={<Rule />} onClick={handleNewRule}>
                                    New Rule
                                </Button>
                            </Stack>
                        }
                    >
                            <Stack spacing={2}>
                                {loadError && <Alert severity="error">{loadError}</Alert>}
                                {actionAlert}

                                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: '1.1fr 1.4fr' }, gap: 2 }}>
                                    <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                                        <Typography variant="subtitle2" sx={{ mb: 1 }}>
                                            Rule List
                                        </Typography>
                                        <List dense>
                                            {rules.length === 0 && (
                                                <ListItem sx={{ px: 0 }}>
                                                    <ListItemText primary="No rules found" secondary="Upload guardrails.yaml to configure rules." />
                                                </ListItem>
                                            )}
                                            {rules.map((rule) => (
                                                <ListItem
                                                    key={rule.id}
                                                    sx={{ px: 0, alignItems: 'flex-start' }}
                                                >
                                                    <Box
                                                        sx={{
                                                            display: 'flex',
                                                            alignItems: 'flex-start',
                                                            width: '100%',
                                                            cursor: 'pointer',
                                                            borderRadius: 1,
                                                            px: 1,
                                                            py: 0.5,
                                                            bgcolor: selectedRuleId === rule.id ? 'action.selected' : 'transparent',
                                                            '&:hover': { bgcolor: 'action.hover' },
                                                        }}
                                                        onClick={() => {
                                                            setSelectedRuleId(rule.id);
                                                            updateEditorFromRule(rule);
                                                        }}
                                                    >
                                                    <ListItemText
                                                        primary={
                                                            <Stack direction="row" spacing={1} alignItems="center">
                                                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                                    {rule.id}
                                                                </Typography>
                                                                <Chip size="small" label={rule.type} variant="outlined" />
                                                            </Stack>
                                                        }
                                                        secondary={
                                                            <Stack spacing={0.5} sx={{ mt: 0.5 }}>
                                                                <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'normal' }}>
                                                                    {rule.reason}
                                                                </Typography>
                                                                <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'normal' }}>
                                                                    {rule.scope}
                                                                </Typography>
                                                            </Stack>
                                                        }
                                                    />
                                                    <Box sx={{ pl: 1, pt: 0.5 }}>
                                                        <Tooltip title={rule.status} arrow>
                                                            <Chip size="small" label={rule.status} />
                                                        </Tooltip>
                                                        <Tooltip title="Delete rule" arrow>
                                                            <span>
                                                                <IconButton
                                                                    size="small"
                                                                    sx={{ ml: 0.5 }}
                                                                    disabled={pendingRuleId === rule.id}
                                                                    onClick={(e) => {
                                                                        e.stopPropagation();
                                                                        setDeleteRuleId(rule.id);
                                                                    }}
                                                                >
                                                                    <DeleteOutline fontSize="small" />
                                                                </IconButton>
                                                            </span>
                                                        </Tooltip>
                                                        <FormControlLabel
                                                            sx={{ ml: 1 }}
                                                            control={
                                                                <Switch
                                                                    size="small"
                                                                    checked={rule.enabled}
                                                                    disabled={pendingRuleId === rule.id}
                                                                    onChange={(e) => handleToggleRule(rule.id, e.target.checked)}
                                                                />
                                                            }
                                                            label="Enabled"
                                                        />
                                                        {pendingRuleId === rule.id && (
                                                            <Chip
                                                                size="small"
                                                                label="Saving…"
                                                                sx={{ ml: 1 }}
                                                            />
                                                        )}
                                                    </Box>
                                                    </Box>
                                                </ListItem>
                                            ))}
                                        </List>
                                    </Box>

                                    <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2 }}>
                                        <Collapse in={editorOpen} unmountOnExit>
                                            <Stack spacing={2}>
                                                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                                    <Typography variant="subtitle2">Rule Editor</Typography>
                                                    <Stack spacing={0} alignItems="flex-end">
                                                        <FormControlLabel
                                                            control={
                                                                <Switch
                                                                    size="small"
                                                                    checked={editorState.enabled}
                                                                    onChange={(e) =>
                                                                        setEditorState((s) => ({ ...s, enabled: e.target.checked }))
                                                                    }
                                                                />
                                                            }
                                                            label="Enabled"
                                                        />
                                                        <Typography variant="caption" color="text.secondary">
                                                            Synced with the rule list toggle.
                                                        </Typography>
                                                    </Stack>
                                                </Box>

                                                <Typography variant="subtitle2">Basic Settings</Typography>
                                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                                                    <TextField
                                                        label="Rule ID"
                                                        size="small"
                                                        fullWidth
                                                        value={editorState.id}
                                                        onChange={(e) => setEditorState((s) => ({ ...s, id: e.target.value }))}
                                                        helperText="Unique identifier used by the engine and API."
                                                    />
                                                    <TextField
                                                        label="Name"
                                                        size="small"
                                                        fullWidth
                                                        value={editorState.name}
                                                        onChange={(e) => setEditorState((s) => ({ ...s, name: e.target.value }))}
                                                        helperText="Human-friendly label shown in UI."
                                                    />
                                                </Stack>

                                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                                                    <FormControl size="small" fullWidth>
                                                        <InputLabel id="rule-type">Rule Type</InputLabel>
                                                        <Select
                                                            labelId="rule-type"
                                                            label="Rule Type"
                                                            value={editorState.type}
                                                            onChange={(e) =>
                                                                setEditorState((s) => ({ ...s, type: String(e.target.value) }))
                                                            }
                                                        >
                                                            <MenuItem value="text_match">text_match</MenuItem>
                                                            <MenuItem value="command_policy">command_policy</MenuItem>
                                                            {editorState.type === 'model_judge' && (
                                                                <MenuItem value="model_judge" disabled>
                                                                    model_judge (legacy)
                                                                </MenuItem>
                                                            )}
                                                        </Select>
                                                        <FormHelperText>
                                                            How the rule evaluates content. `model_judge` is hidden from new rules for now.
                                                        </FormHelperText>
                                                    </FormControl>
                                                    <FormControl size="small" fullWidth>
                                                        <InputLabel id="rule-verdict">Default Verdict</InputLabel>
                                                        <Select
                                                            labelId="rule-verdict"
                                                            label="Default Verdict"
                                                            value={editorState.verdict}
                                                            onChange={(e) =>
                                                                setEditorState((s) => ({ ...s, verdict: String(e.target.value) }))
                                                            }
                                                        >
                                                            <MenuItem value="allow">allow</MenuItem>
                                                            <MenuItem value="review">review</MenuItem>
                                                            <MenuItem value="block">block</MenuItem>
                                                        </Select>
                                                        <FormHelperText>Action to take when the rule matches.</FormHelperText>
                                                    </FormControl>
                                                </Stack>

                                                {editorState.type === 'command_policy' ? (
                                                    <Stack spacing={2}>
                                                        <Box>
                                                            <Typography variant="subtitle2">Command Policy</Typography>
                                                            <Typography variant="caption" color="text.secondary">
                                                                Define what kind of command behavior should be blocked, such as reading
                                                                sensitive directories.
                                                            </Typography>
                                                        </Box>

                                                        <Box>
                                                            <Typography variant="caption" color="text.secondary">
                                                                Command Kind
                                                            </Typography>
                                                            <FormGroup row sx={{ mt: 0.5, columnGap: 1.5, rowGap: 0.5 }}>
                                                                {commandKindOptions.map((option) => (
                                                                    <FormControlLabel
                                                                        key={`kind-${option}`}
                                                                        sx={{ ml: 0, mr: 1 }}
                                                                        control={
                                                                            <Checkbox
                                                                                size="small"
                                                                                checked={editorState.kinds.includes(option)}
                                                                                onChange={() =>
                                                                                    setEditorState((state) => ({
                                                                                        ...state,
                                                                                        kinds: toggleValue(state.kinds, option),
                                                                                    }))
                                                                                }
                                                                            />
                                                                        }
                                                                        label={option}
                                                                    />
                                                                ))}
                                                            </FormGroup>
                                                            <FormHelperText>Usually `shell` for Claude Code bash calls.</FormHelperText>
                                                        </Box>

                                                        <Box>
                                                            <Typography variant="caption" color="text.secondary">
                                                                Actions
                                                            </Typography>
                                                            <FormGroup row sx={{ mt: 0.5, columnGap: 1.5, rowGap: 0.5 }}>
                                                                {commandActionOptions.map((option) => (
                                                                    <FormControlLabel
                                                                        key={`action-${option}`}
                                                                        sx={{ ml: 0, mr: 1 }}
                                                                        control={
                                                                            <Checkbox
                                                                                size="small"
                                                                                checked={editorState.actions.includes(option)}
                                                                                onChange={() =>
                                                                                    setEditorState((state) => ({
                                                                                        ...state,
                                                                                        actions: toggleValue(state.actions, option),
                                                                                    }))
                                                                                }
                                                                            />
                                                                        }
                                                                        label={option}
                                                                    />
                                                                ))}
                                                            </FormGroup>
                                                            <FormHelperText>
                                                                Example: use `read` to block reading `~/.ssh`.
                                                            </FormHelperText>
                                                        </Box>

                                                        <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                                                            <TextField
                                                                label="Resources"
                                                                size="small"
                                                                fullWidth
                                                                multiline
                                                                minRows={3}
                                                                value={editorState.resources}
                                                                onChange={(e) =>
                                                                    setEditorState((s) => ({ ...s, resources: e.target.value }))
                                                                }
                                                                helperText="One path/resource per line, for example `~/.ssh` or `/etc/ssh`."
                                                            />
                                                            <Stack spacing={2} sx={{ minWidth: { md: 220 } }}>
                                                                <FormControl size="small" fullWidth>
                                                                    <InputLabel id="resource-match">Resource Match</InputLabel>
                                                                    <Select
                                                                        labelId="resource-match"
                                                                        label="Resource Match"
                                                                        value={editorState.resourceMatch}
                                                                        onChange={(e) =>
                                                                            setEditorState((s) => ({
                                                                                ...s,
                                                                                resourceMatch: String(e.target.value),
                                                                            }))
                                                                        }
                                                                    >
                                                                        <MenuItem value="prefix">prefix</MenuItem>
                                                                        <MenuItem value="contains">contains</MenuItem>
                                                                        <MenuItem value="exact">exact</MenuItem>
                                                                    </Select>
                                                                    <FormHelperText>
                                                                        `prefix` is usually the safest default for paths.
                                                                    </FormHelperText>
                                                                </FormControl>
                                                            </Stack>
                                                        </Stack>
                                                    </Stack>
                                                ) : (
                                                    <TextField
                                                        label="Patterns"
                                                        size="small"
                                                        fullWidth
                                                        multiline
                                                        minRows={3}
                                                        value={editorState.patterns}
                                                        onChange={(e) => setEditorState((s) => ({ ...s, patterns: e.target.value }))}
                                                        helperText="One pattern per line. Used by text_match rules."
                                                    />
                                                )}

                                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} alignItems={{ md: 'flex-start' }}>
                                                    <TextField
                                                        label="Reason"
                                                        size="small"
                                                        fullWidth
                                                        value={editorState.reason}
                                                        onChange={(e) => setEditorState((s) => ({ ...s, reason: e.target.value }))}
                                                        helperText="Shown to users when a rule blocks or reviews content."
                                                    />
                                                    <Button
                                                        variant="outlined"
                                                        size="small"
                                                        sx={{ minWidth: { md: 140 }, mt: { md: 0.5 } }}
                                                        onClick={() =>
                                                            setEditorState((state) => ({
                                                                ...state,
                                                                reason: buildSuggestedReason(state),
                                                            }))
                                                        }
                                                    >
                                                        Generate
                                                    </Button>
                                                </Stack>

                                                <Accordion
                                                    defaultExpanded={false}
                                                    elevation={0}
                                                    sx={{ border: '1px solid', borderColor: 'divider' }}
                                                >
                                                    <AccordionSummary expandIcon={<ExpandMore />}>
                                                        <Stack>
                                                            <Typography variant="subtitle2">Advanced Settings</Typography>
                                                            <Typography variant="caption" color="text.secondary">
                                                                Scope and evaluation targets. Defaults apply when left empty.
                                                            </Typography>
                                                        </Stack>
                                                    </AccordionSummary>
                                                    <AccordionDetails>
                                                        <Stack spacing={2}>
                                                            <Box>
                                                                <Typography variant="subtitle2">Scope</Typography>
                                                                <Typography variant="caption" color="text.secondary">
                                                                    Where the rule applies: scenario (provider), direction, and content
                                                                    type.
                                                                </Typography>
                                                                <Stack spacing={1.5} sx={{ mt: 1 }}>
                                                                    <FormControl component="fieldset" variant="standard">
                                                                        <Typography variant="caption" color="text.secondary">
                                                                            Scenarios
                                                                        </Typography>
                                                                        <FormGroup
                                                                            row
                                                                            sx={{
                                                                                alignItems: 'center',
                                                                                columnGap: 1.5,
                                                                                rowGap: 0.5,
                                                                            }}
                                                                        >
                                                                            {scenarioOptions.map((option) => (
                                                                                <FormControlLabel
                                                                                    key={`scenario-${option}`}
                                                                                    sx={{ ml: 0, mr: 1 }}
                                                                                    control={
                                                                                        <Checkbox
                                                                                            size="small"
                                                                                            checked={editorState.scenarios.includes(option)}
                                                                                            onChange={() =>
                                                                                                setEditorState((state) => ({
                                                                                                    ...state,
                                                                                                    scenarios: toggleValue(
                                                                                                        state.scenarios,
                                                                                                        option
                                                                                                    ),
                                                                                                }))
                                                                                            }
                                                                                        />
                                                                                    }
                                                                                    label={option}
                                                                                />
                                                                            ))}
                                                                        </FormGroup>
                                                                        <FormHelperText>
                                                                            Leave empty to apply to all scenarios.
                                                                        </FormHelperText>
                                                                    </FormControl>

                                                                    <FormControl component="fieldset" variant="standard">
                                                                        <Typography variant="caption" color="text.secondary">
                                                                            Directions
                                                                        </Typography>
                                                                        <FormGroup
                                                                            row
                                                                            sx={{
                                                                                alignItems: 'center',
                                                                                columnGap: 1.5,
                                                                                rowGap: 0.5,
                                                                            }}
                                                                        >
                                                                            {directionOptions.map((option) => (
                                                                                <FormControlLabel
                                                                                    key={`direction-${option}`}
                                                                                    sx={{ ml: 0, mr: 1 }}
                                                                                    control={
                                                                                        <Checkbox
                                                                                            size="small"
                                                                                            checked={editorState.directions.includes(option)}
                                                                                            onChange={() =>
                                                                                                setEditorState((state) => ({
                                                                                                    ...state,
                                                                                                    directions: toggleValue(
                                                                                                        state.directions,
                                                                                                        option
                                                                                                    ),
                                                                                                }))
                                                                                            }
                                                                                        />
                                                                                    }
                                                                                    label={option}
                                                                                />
                                                                            ))}
                                                                        </FormGroup>
                                                                        <FormHelperText>
                                                                            Request = tool_result, Response = model output.
                                                                        </FormHelperText>
                                                                    </FormControl>

                                                                    <FormControl component="fieldset" variant="standard">
                                                                        <Typography variant="caption" color="text.secondary">
                                                                            Content Types
                                                                        </Typography>
                                                                        <FormGroup
                                                                            row
                                                                            sx={{
                                                                                alignItems: 'center',
                                                                                columnGap: 1.5,
                                                                                rowGap: 0.5,
                                                                            }}
                                                                        >
                                                                            {contentTypeOptions.map((option) => (
                                                                                <FormControlLabel
                                                                                    key={`content-${option}`}
                                                                                    sx={{ ml: 0, mr: 1 }}
                                                                                    control={
                                                                                        <Checkbox
                                                                                            size="small"
                                                                                            checked={editorState.contentTypes.includes(option)}
                                                                                            onChange={() =>
                                                                                                setEditorState((state) => ({
                                                                                                    ...state,
                                                                                                    contentTypes: toggleValue(
                                                                                                        state.contentTypes,
                                                                                                        option
                                                                                                    ),
                                                                                                }))
                                                                                            }
                                                                                        />
                                                                                    }
                                                                                    label={option}
                                                                                />
                                                                            ))}
                                                                        </FormGroup>
                                                                        <FormHelperText>
                                                                            Controls which content is eligible for this rule.
                                                                        </FormHelperText>
                                                                    </FormControl>
                                                                </Stack>
                                                            </Box>

                                                            {editorState.type !== 'command_policy' && (
                                                                <Box sx={{ width: '100%' }}>
                                                                    <Typography variant="subtitle2">Targets</Typography>
                                                                    <Typography variant="caption" color="text.secondary">
                                                                        Which content parts the rule evaluates once it runs.
                                                                    </Typography>
                                                                    <FormControl
                                                                        component="fieldset"
                                                                        variant="standard"
                                                                        sx={{ mt: 1, alignItems: 'flex-start', width: '100%' }}
                                                                    >
                                                                        <FormGroup
                                                                            row
                                                                            sx={{
                                                                                alignItems: 'center',
                                                                                columnGap: 1.5,
                                                                                rowGap: 0.5,
                                                                                justifyContent: 'flex-start',
                                                                                width: '100%',
                                                                            }}
                                                                        >
                                                                            {targetOptions.map((option) => (
                                                                                <FormControlLabel
                                                                                    key={`target-${option}`}
                                                                                    sx={{ ml: 0, mr: 1 }}
                                                                                    control={
                                                                                        <Checkbox
                                                                                            size="small"
                                                                                            checked={editorState.targets.includes(option)}
                                                                                            onChange={() =>
                                                                                                setEditorState((state) => ({
                                                                                                    ...state,
                                                                                                    targets: toggleValue(state.targets, option),
                                                                                                }))
                                                                                            }
                                                                                        />
                                                                                    }
                                                                                    label={option}
                                                                                />
                                                                            ))}
                                                                        </FormGroup>
                                                                        <FormHelperText sx={{ textAlign: 'left' }}>
                                                                            Leave empty to evaluate all available content.
                                                                        </FormHelperText>
                                                                    </FormControl>
                                                                </Box>
                                                            )}
                                                        </Stack>
                                                    </AccordionDetails>
                                                </Accordion>

                                                <Stack direction="row" spacing={1} justifyContent="flex-end">
                                                    <Button variant="outlined" size="small" onClick={handleCloseEditor}>
                                                        Close
                                                    </Button>
                                                    <Button
                                                        variant="outlined"
                                                        size="small"
                                                        disabled={pendingSave}
                                                        onClick={handleDuplicateRule}
                                                    >
                                                        Duplicate
                                                    </Button>
                                                    <Button
                                                        variant="contained"
                                                        size="small"
                                                        disabled={pendingSave}
                                                        onClick={handleSaveRule}
                                                    >
                                                        {pendingSave ? 'Saving…' : 'Save'}
                                                    </Button>
                                                </Stack>
                                            </Stack>
                                        </Collapse>
                                        {!editorOpen && (
                                            <Box sx={{ py: 6, textAlign: 'center' }}>
                                                <Typography variant="body2" color="text.secondary">
                                                    Select a rule from the list or create a new one to start editing.
                                                </Typography>
                                            </Box>
                                        )}
                                        <Dialog open={confirmCloseOpen} onClose={() => handleConfirmClose('cancel')}>
                                            <DialogTitle>Unsaved changes</DialogTitle>
                                            <DialogContent>
                                                <Typography variant="body2" color="text.secondary">
                                                    You have unsaved changes in this rule. What would you like to do?
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
                                        <Dialog open={!!deleteRuleId} onClose={() => setDeleteRuleId(null)}>
                                            <DialogTitle>Delete rule</DialogTitle>
                                            <DialogContent>
                                                <Typography variant="body2" color="text.secondary">
                                                    {deleteRuleId
                                                        ? `Delete rule "${deleteRuleId}"? This will update guardrails.yaml and reload the engine.`
                                                        : 'Delete this rule?'}
                                                </Typography>
                                            </DialogContent>
                                            <DialogActions>
                                                <Button variant="text" onClick={() => setDeleteRuleId(null)}>
                                                    Cancel
                                                </Button>
                                                <Button variant="contained" color="error" onClick={handleDeleteRule}>
                                                    Delete
                                                </Button>
                                            </DialogActions>
                                        </Dialog>
                                    </Box>
                                </Box>

                            </Stack>
                    </UnifiedCard>
                </Box>
            </Stack>
        </PageLayout>
    );
};

export default GuardrailsRulesPage;
