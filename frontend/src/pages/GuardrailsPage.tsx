import { useEffect, useMemo, useRef, useState } from 'react';
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
} from '@mui/material';
import { Rule, Tune, ExpandMore } from '@mui/icons-material';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

type GuardrailsRule = {
    id: string;
    type: string;
    scope: string;
    status: string;
    reason: string;
    enabled: boolean;
};

const GuardrailsPage = () => {
    const [loading, setLoading] = useState(true);
    const [rules, setRules] = useState<GuardrailsRule[]>([]);
    const [rawRules, setRawRules] = useState<any[]>([]);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [configContent, setConfigContent] = useState<string>('');
    const [actionMessage, setActionMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [pendingRuleId, setPendingRuleId] = useState<string | null>(null);
    const [pendingSave, setPendingSave] = useState(false);
    const [selectedRuleId, setSelectedRuleId] = useState<string | null>(null);
    const [isNewRule, setIsNewRule] = useState(false);
    const [editorOpen, setEditorOpen] = useState(false);
    const [editorSnapshot, setEditorSnapshot] = useState('');
    const [confirmCloseOpen, setConfirmCloseOpen] = useState(false);
    const [editorState, setEditorState] = useState({
        id: '',
        name: '',
        type: 'text_match',
        verdict: 'block',
        enabled: true,
        scenarios: ['anthropic'],
        directions: ['response'],
        contentTypes: ['command'],
        patterns: '',
        reason: '',
        targets: ['command'],
    });

    const isEditorDirty = useMemo(() => {
        if (!editorSnapshot) {
            return false;
        }
        return JSON.stringify(editorState) !== editorSnapshot;
    }, [editorState, editorSnapshot]);

    const scenarioOptions = useMemo(() => {
        const fromRules = rawRules.flatMap((rule) => rule?.scope?.scenarios ?? []);
        return Array.from(new Set(['anthropic', 'claude_code', 'openai', ...fromRules])).filter(Boolean);
    }, [rawRules]);

    const directionOptions = useMemo(() => {
        const fromRules = rawRules.flatMap((rule) => rule?.scope?.directions ?? []);
        return Array.from(new Set(['request', 'response', ...fromRules])).filter(Boolean);
    }, [rawRules]);

    const contentTypeOptions = useMemo(() => {
        const fromRules = rawRules.flatMap((rule) => rule?.scope?.content_types ?? []);
        return Array.from(new Set(['command', 'text', 'messages', ...fromRules])).filter(Boolean);
    }, [rawRules]);

    const targetOptions = contentTypeOptions;

    const toggleValue = (values: string[], value: string) => {
        if (values.includes(value)) {
            return values.filter((item) => item !== value);
        }
        return [...values, value];
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
                setConfigContent(guardrailsConfig?.content || '');
                setLoadError(null);
            } catch (error) {
                console.error('Failed to load guardrails flags:', error);
                setRules([]);
                setRawRules([]);
                setConfigContent('');
                setLoadError('Failed to load guardrails config');
            } finally {
                setLoading(false);
            }
        };
        loadFlags();
    }, []);

    const mapRules = (rawRules: any[]): GuardrailsRule[] => {
        return rawRules.map((rule: any) => {
            const scope = rule.scope || {};
            const contentTypes = Array.isArray(scope.content_types) ? scope.content_types.join(', ') : 'all';
            const directions = Array.isArray(scope.directions) ? scope.directions.join(', ') : 'all';
            const scenarios = Array.isArray(scope.scenarios) ? scope.scenarios.join(', ') : 'all';
            const scopeText = `${contentTypes} · ${directions} · ${scenarios}`;
            return {
                id: rule.id || 'unknown',
                type: rule.type || 'unknown',
                scope: scopeText,
                status: rule.enabled ? 'Enabled' : 'Disabled',
                reason: (rule.params && rule.params.reason) || 'n/a',
                enabled: !!rule.enabled,
            };
        });
    };

    const updateEditorFromRule = (rule: GuardrailsRule) => {
        const rawRule = rawRules.find((r) => r.id === rule.id) || {};
        const scope = rawRule.scope || {};
        const params = rawRule.params || {};
        const patterns = Array.isArray(params.patterns) ? params.patterns.join('\n') : '';
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
            setConfigContent(guardrailsConfig?.content || '');
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
            const payload = {
                id: editorState.id,
                name: editorState.name,
                type: editorState.type,
                enabled: editorState.enabled,
                scope: {
                    scenarios: editorState.scenarios,
                    directions: editorState.directions,
                    content_types: editorState.contentTypes,
                },
                params: {
                    patterns: editorState.patterns
                        .split('\n')
                        .map((p) => p.trim())
                        .filter(Boolean),
                    verdict: editorState.verdict,
                    reason: editorState.reason,
                    targets: editorState.targets,
                },
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
            setConfigContent(guardrailsConfig?.content || '');
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
            type: 'text_match',
            verdict: 'block',
            enabled: true,
            scenarios: ['anthropic'],
            directions: ['response'],
            contentTypes: ['command'],
            patterns: '',
            reason: '',
            targets: ['command'],
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
                params: {
                    patterns: editorState.patterns
                        .split('\n')
                        .map((p) => p.trim())
                        .filter(Boolean),
                    verdict: editorState.verdict,
                    reason: editorState.reason,
                    targets: editorState.targets,
                },
            };
            const result = await api.createGuardrailsRule(payload);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to duplicate rule' });
                return;
            }
            const guardrailsConfig = await api.getGuardrailsConfig();
            setRules(guardrailsConfig?.config?.rules ? mapRules(guardrailsConfig.config.rules) : []);
            setConfigContent(guardrailsConfig?.content || '');
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
            setConfigContent(guardrailsConfig?.content || '');
            setActionMessage({ type: 'success', text: 'Guardrails reloaded successfully.' });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to reload guardrails config' });
        } finally {
            setLoading(false);
        }
    };

    const handleImportClick = () => {
        fileInputRef.current?.click();
    };

    const handleImportFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) {
            return;
        }
        try {
            setLoading(true);
            const content = await file.text();
            const result = await api.updateGuardrailsConfig(content);
            if (!result?.success) {
                setActionMessage({ type: 'error', text: result?.error || 'Failed to update guardrails config' });
                return;
            }
            const guardrailsConfig = await api.getGuardrailsConfig();
            setRules(guardrailsConfig?.config?.rules ? mapRules(guardrailsConfig.config.rules) : []);
            setConfigContent(guardrailsConfig?.content || '');
            setActionMessage({ type: 'success', text: 'Guardrails config updated.' });
        } catch (error: any) {
            setActionMessage({ type: 'error', text: error?.message || 'Failed to update guardrails config' });
        } finally {
            setLoading(false);
            if (fileInputRef.current) {
                fileInputRef.current.value = '';
            }
        }
    };

    const handleExport = () => {
        if (!configContent) {
            setActionMessage({ type: 'error', text: 'No guardrails config content available to export.' });
            return;
        }
        const blob = new Blob([configContent], { type: 'text/yaml' });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = 'guardrails.yaml';
        link.click();
        URL.revokeObjectURL(url);
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

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title="Guardrails"
                    subtitle="Manage rule-based safety checks for tool calls and tool results."
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Button variant="outlined" size="small" startIcon={<Tune />} onClick={handleReload}>
                                Reload
                            </Button>
                        </Stack>
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
                    <UnifiedCard title="Rules" size="full">
                            <Stack spacing={2}>
                                {loadError && <Alert severity="error">{loadError}</Alert>}
                                {actionAlert}
                                <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
                                    <Box sx={{ flex: 1 }}>
                                        <TextField
                                            fullWidth
                                            size="small"
                                            label="Search rules"
                                            placeholder="Search by id, name, type"
                                            disabled
                                        />
                                    </Box>
                                    <Stack direction="row" spacing={1}>
                                        <Button variant="outlined" size="small" startIcon={<Rule />} onClick={handleNewRule}>
                                            New Rule
                                        </Button>
                                        <input
                                            ref={fileInputRef}
                                            type="file"
                                            accept=".yaml,.yml,.json"
                                            style={{ display: 'none' }}
                                            onChange={handleImportFile}
                                        />
                                        <Button variant="outlined" size="small" onClick={handleImportClick}>
                                            Import
                                        </Button>
                                        <Button variant="outlined" size="small" onClick={handleExport}>
                                            Export
                                        </Button>
                                    </Stack>
                                </Stack>

                                <Divider />

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
                                                            <MenuItem value="model_judge">model_judge</MenuItem>
                                                        </Select>
                                                        <FormHelperText>How the rule evaluates content.</FormHelperText>
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

                                                <TextField
                                                    label="Reason"
                                                    size="small"
                                                    fullWidth
                                                    value={editorState.reason}
                                                    onChange={(e) => setEditorState((s) => ({ ...s, reason: e.target.value }))}
                                                    helperText="Shown to users when a rule blocks or reviews content."
                                                />

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
                                    </Box>
                                </Box>

                            </Stack>
                    </UnifiedCard>
                </Box>
            </Stack>
        </PageLayout>
    );
};

export default GuardrailsPage;
