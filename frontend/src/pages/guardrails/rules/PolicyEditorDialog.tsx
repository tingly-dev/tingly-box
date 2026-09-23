import { useEffect, useMemo, useState } from 'react';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    FormControlLabel,
    FormHelperText,
    IconButton,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import {
    CheckCircleRounded,
    ExpandMore,
    HelpOutline,
} from '@/components/icons';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import { blurActiveElement } from '@/utils/dom';
import CompactListEditor from './CompactListEditor';
import PolicyKindCards from './PolicyKindCards';
import ScenarioScopeSelector from './ScenarioScopeSelector';
import {
    applyKindDefaults,
    buildPolicyPayload,
    buildSuggestedReason,
    generatePolicyId,
    uniqueIdFromName,
} from './editorState';
import { effectiveListValues, splitLines, toggleValue } from './listField';
import {
    commandExecutionActionOptions,
    resourceAccessActionOptions,
    type EditorListField,
    type EditorState,
    type GuardrailsPolicy,
    type OversizedListField,
} from './types';

// Describes how the editor was opened. Built by the page for each open intent
// (list click, New Policy, deep link, location-state draft).
export type PolicyEditorSession = {
    policyId: string | null;
    isNewPolicy: boolean;
    state: EditorState;
    oversized: Partial<Record<EditorListField, OversizedListField>>;
    // Row selections seeded by the open path. Undefined means "keep whatever
    // row selection the dialog already had" (matches the historical deep-link
    // behavior, which never reset these).
    rows?: { resourceRow: number; commandTermRow: number; patternRow: number };
};

type PolicyEditorDialogProps = {
    open: boolean;
    session: PolicyEditorSession | null;
    policies: GuardrailsPolicy[];
    groupOptions: Array<{ value: string; label: string }>;
    scenarioOptions: string[];
    selectedPolicyId: string | null;
    onSelectedPolicyIdChange: (policyId: string | null) => void;
    onClose: () => void;
    onReloadPolicies: () => Promise<void>;
};

// The policy editor dialog owns all editor-local state (draft, snapshot/dirty
// tracking, row selections, advanced panel, confirm-close machine). Keeping it
// out of the page means every keystroke re-renders only the dialog.
const PolicyEditorDialog = ({
    open,
    session,
    policies,
    groupOptions,
    scenarioOptions,
    selectedPolicyId,
    onSelectedPolicyIdChange,
    onClose,
    onReloadPolicies,
}: PolicyEditorDialogProps) => {
    const notify = useNotify();
    const [editorState, setEditorState] = useState<EditorState>({
        id: '',
        name: '',
        groups: [],
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
    const [editorSnapshot, setEditorSnapshot] = useState('');
    const [isNewPolicy, setIsNewPolicy] = useState(false);
    const [oversizedListFields, setOversizedListFields] = useState<Partial<Record<EditorListField, OversizedListField>>>({});
    const [selectedResourceRow, setSelectedResourceRow] = useState(-1);
    const [selectedCommandTermRow, setSelectedCommandTermRow] = useState(-1);
    const [selectedPatternRow, setSelectedPatternRow] = useState(-1);
    const [advancedOpen, setAdvancedOpen] = useState(false);
    const [confirmCloseOpen, setConfirmCloseOpen] = useState(false);
    const [pendingSave, setPendingSave] = useState(false);

    useEffect(() => {
        if (!open || !session) {
            return;
        }
        setEditorState(session.state);
        setEditorSnapshot(JSON.stringify(session.state));
        setIsNewPolicy(session.isNewPolicy);
        setOversizedListFields(session.oversized);
        if (session.rows) {
            setSelectedResourceRow(session.rows.resourceRow);
            setSelectedCommandTermRow(session.rows.commandTermRow);
            setSelectedPatternRow(session.rows.patternRow);
        }
    }, [open, session]);

    const isEditorDirty = useMemo(() => {
        if (!editorSnapshot) {
            return false;
        }
        return JSON.stringify(editorState) !== editorSnapshot;
    }, [editorState, editorSnapshot]);

    const handleSelectPolicyGroup = (groupID: string) => {
        setEditorState((state) => ({
            ...state,
            groups: state.groups.includes(groupID) ? state.groups.filter((value) => value !== groupID) : [...state.groups, groupID],
        }));
    };

    const handleSelectPolicyKind = (kind: 'resource_access' | 'command_execution' | 'content') => {
        setEditorState((state) =>
            applyKindDefaults(kind, state, { isNewPolicy, takenIds: policies.map((policy) => policy.id) })
        );
    };

    const handleSavePolicy = async (): Promise<boolean> => {
        if (!editorState.kind) {
            notify.error('Choose a policy kind first.');
            return false;
        }
        if (!editorState.name.trim()) {
            notify.error('Policy name is required before saving.');
            return false;
        }
        const takenIds = policies.map((policy) => policy.id);
        const effectiveEditorState =
            editorState.id.trim() || !editorState.kind
                ? editorState
                : {
                      ...editorState,
                      id: generatePolicyId(editorState.name, editorState.kind, takenIds, isNewPolicy ? undefined : selectedPolicyId || editorState.id),
                  };
        if (editorState.kind === 'content' && effectiveListValues(oversizedListFields, 'patterns', editorState.patterns).length === 0) {
            notify.error('Privacy policies require at least one pattern.');
            return false;
        }
        if (
            editorState.kind === 'resource_access' &&
            effectiveListValues(oversizedListFields, 'resources', editorState.resources).length === 0 &&
            editorState.actions.length === 0 &&
            effectiveListValues(oversizedListFields, 'toolNames', editorState.toolNames).length === 0
        ) {
            notify.error('Resource access policies require at least one action, resource, or tool filter.');
            return false;
        }
        if (
            editorState.kind === 'command_execution' &&
            effectiveListValues(oversizedListFields, 'commandTerms', editorState.commandTerms).length === 0 &&
            effectiveListValues(oversizedListFields, 'toolNames', editorState.toolNames).length === 0 &&
            effectiveListValues(oversizedListFields, 'resources', editorState.resources).length === 0
        ) {
            notify.error('Command execution policies require a term match, tool filter, or resource filter.');
            return false;
        }

        try {
            setPendingSave(true);
            const payload = buildPolicyPayload(effectiveEditorState, oversizedListFields);
            const targetPolicyId = isNewPolicy ? effectiveEditorState.id : (selectedPolicyId || effectiveEditorState.id);
            const result = isNewPolicy
                ? await api.createGuardrailsPolicy(payload)
                : await api.updateGuardrailsPolicy(targetPolicyId, payload);
            if (!result?.success) {
                notify.error(result?.error || 'Failed to save policy');
                return false;
            }
            await onReloadPolicies();
            setEditorState(effectiveEditorState);
            onSelectedPolicyIdChange(effectiveEditorState.id);
            setIsNewPolicy(false);
            setEditorSnapshot(JSON.stringify(effectiveEditorState));
            notify.success(`Policy "${effectiveEditorState.id}" saved.`);
            onClose();
            setConfirmCloseOpen(false);
            return true;
        } catch (error: any) {
            notify.error(error?.message || 'Failed to save policy');
            return false;
        } finally {
            setPendingSave(false);
        }
    };

    const handleDuplicatePolicy = () => {
        const nextId = uniqueIdFromName('', policies.map((policy) => policy.id), `${editorState.id}-copy`);

        const nextState = {
            ...editorState,
            id: nextId,
            name: `${editorState.name} (copy)`,
        };

        // Duplicating now only creates a local draft. The copied policy is not
        // persisted until the user explicitly saves it.
        setIsNewPolicy(true);
        onSelectedPolicyIdChange(null);
        setSelectedResourceRow(splitLines(nextState.resources).length > 0 ? 0 : -1);
        setSelectedCommandTermRow(splitLines(nextState.commandTerms).length > 0 ? 0 : -1);
        setSelectedPatternRow(splitLines(nextState.patterns).length > 0 ? 0 : -1);
        setEditorState(nextState);
        setEditorSnapshot(JSON.stringify(editorState));
        notify.success(`Draft copy "${nextId}" is ready. Save to create it.`);
    };

    const handleCloseEditor = () => {
        if (isEditorDirty) {
            setConfirmCloseOpen(true);
            return;
        }
        onClose();
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
        onClose();
        blurActiveElement();
    };

    return (
        <>
        <Dialog open={open} onClose={handleCloseEditor} disableRestoreFocus fullWidth maxWidth="md">
            <DialogTitle>{isNewPolicy ? 'New Policy' : `Edit Policy${selectedPolicyId ? ` · ${selectedPolicyId}` : ''}`}</DialogTitle>
            <DialogContent dividers>
                <Stack spacing={2} sx={{ pt: 1 }}>
                    <Stack spacing={1}>
                        <Stack direction="row" spacing={0.75} sx={{
                            alignItems: "center"
                        }}>
                            <Typography variant="subtitle2">Basic Settings</Typography>
                            <Tooltip title="Choose the policy type first, then fill in only the fields that apply to that type.">
                                <IconButton size="small" sx={{ p: 0.25 }}>
                                    <HelpOutline fontSize="inherit" />
                                </IconButton>
                            </Tooltip>
                        </Stack>
                        <PolicyKindCards kind={editorState.kind} onSelect={handleSelectPolicyKind} />
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
                                        id: isNewPolicy ? generatePolicyId(name, state.kind, policies.map((policy) => policy.id)) : state.id,
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
                                                        <Stack
                                                            direction="row"
                                                            spacing={0.75}
                                                            useFlexGap
                                                            sx={{
                                                                alignItems: "center",
                                                                flexWrap: "wrap"
                                                            }}>
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
                                            <Stack direction="row" spacing={0.75} sx={{
                                                alignItems: "center"
                                            }}>
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
                                                                <Stack
                                                                    direction="row"
                                                                    spacing={1}
                                                                    useFlexGap
                                                                    sx={{
                                                                        alignItems: "center",
                                                                        flexWrap: "wrap"
                                                                    }}>
                                                                    <Typography variant="body2" sx={{
                                                                        fontWeight: 600
                                                                    }}>
                                                                        {option.label}
                                                                    </Typography>
                                                                    {selected && (
                                                                        <Tooltip title="Selected">
                                                                            <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                                        </Tooltip>
                                                                    )}
                                                                </Stack>
                                                                <Typography variant="caption" sx={{
                                                                    color: "text.secondary"
                                                                }}>
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
                                            <CompactListEditor
                                                title="Protected Resources"
                                                description="Define the files, directories, URLs, or other resources this policy protects."
                                                columnLabel="Path / URL / Resource"
                                                value={editorState.resources}
                                                oversizedField={oversizedListFields.resources}
                                                selectedIndex={selectedResourceRow}
                                                onSelectedIndexChange={setSelectedResourceRow}
                                                onChange={(resources) => setEditorState((state) => ({ ...state, resources }))}
                                                placeholder="~/.ssh"
                                                helperText="Add one resource per row, such as `~/.ssh`, `.env`, `/etc/ssh`, or `https://api.example.com`."
                                            />
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
                                            <Stack direction="row" spacing={0.75} sx={{
                                                alignItems: "center"
                                            }}>
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
                                                                <Stack
                                                                    direction="row"
                                                                    spacing={1}
                                                                    useFlexGap
                                                                    sx={{
                                                                        alignItems: "center",
                                                                        flexWrap: "wrap"
                                                                    }}>
                                                                    <Typography variant="body2" sx={{
                                                                        fontWeight: 600
                                                                    }}>
                                                                        {option.label}
                                                                    </Typography>
                                                                    {selected && (
                                                                        <Tooltip title="Selected">
                                                                            <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                                        </Tooltip>
                                                                    )}
                                                                </Stack>
                                                                <Typography variant="caption" sx={{
                                                                    color: "text.secondary"
                                                                }}>
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
                                        <CompactListEditor
                                            title={editorState.actions.includes('install') ? 'Install Match' : 'Command Match'}
                                            description={
                                                editorState.actions.includes('install')
                                                    ? 'Describe the install targets you want to block or review. This matches normalized command terms such as package, crate, gem, or extension names.'
                                                    : 'Describe the command patterns you want to block or review. This is the main selector for execute policies.'
                                            }
                                            columnLabel={editorState.actions.includes('install') ? 'Install Term' : 'Command Pattern'}
                                            value={editorState.commandTerms}
                                            oversizedField={oversizedListFields.commandTerms}
                                            selectedIndex={selectedCommandTermRow}
                                            onSelectedIndexChange={setSelectedCommandTermRow}
                                            onChange={(commandTerms) => setEditorState((state) => ({ ...state, commandTerms }))}
                                            placeholder={editorState.actions.includes('install') ? 'left-pad' : 'rm -rf'}
                                            helperText={editorState.actions.includes('install')
                                                ? 'One term per row, such as `left-pad`, `requests`, `ripgrep`, or `ms-python.python`.'
                                                : 'One pattern per row, such as `rm -rf`, `curl | sh`, or `python -c`.'}
                                        />
                                    </Box>

                                    <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 2, gridColumn: { md: '1 / span 2' } }}>
                                        <Stack spacing={1.5}>
                                            <CompactListEditor
                                                title="Limit To Resources"
                                                description={
                                                    editorState.actions.includes('install')
                                                        ? 'Optional. Add paths or resources only when the install rule should be limited to specific targets.'
                                                        : 'Optional. Add paths only when the command rule should apply to a specific file, directory, URL, or other resource.'
                                                }
                                                columnLabel="Path / Resource"
                                                value={editorState.resources}
                                                oversizedField={oversizedListFields.resources}
                                                selectedIndex={selectedResourceRow}
                                                onSelectedIndexChange={setSelectedResourceRow}
                                                onChange={(resources) => setEditorState((state) => ({ ...state, resources }))}
                                                placeholder="~/.ssh"
                                                helperText="Optional. Add one resource per row."
                                            />
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
                                            <CompactListEditor
                                                title="Content Patterns"
                                                description="Define the text you want to block or review. Each row becomes one pattern."
                                                columnLabel="Pattern"
                                                value={editorState.patterns}
                                                oversizedField={oversizedListFields.patterns}
                                                selectedIndex={selectedPatternRow}
                                                onSelectedIndexChange={setSelectedPatternRow}
                                                onChange={(patterns) => setEditorState((state) => ({ ...state, patterns }))}
                                                placeholder="BEGIN OPENSSH PRIVATE KEY"
                                                helperText="Use a few specific patterns instead of a long generic list."
                                            />
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
                                        sx={{
                                            justifyContent: "space-between",
                                            alignItems: { xs: 'stretch', md: 'flex-start' }
                                        }}>
                                        <Box>
                                            <Typography variant="subtitle2">Reason</Typography>
                                            <Typography
                                                variant="caption"
                                                sx={{
                                                    color: "text.secondary",
                                                    display: 'block',
                                                    mt: 0.5
                                                }}>
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
                                        <Typography variant="caption" sx={{
                                            color: "text.secondary"
                                        }}>
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
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            color: "text.secondary",
                                                            display: 'block',
                                                            mt: 0.5
                                                        }}>
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
                                                                            <Stack
                                                                                direction="row"
                                                                                spacing={1}
                                                                                useFlexGap
                                                                                sx={{
                                                                                    alignItems: "center",
                                                                                    flexWrap: "wrap"
                                                                                }}>
                                                                                <Typography variant="body2" sx={{
                                                                                    fontWeight: 600
                                                                                }}>
                                                                                    {option.label}
                                                                                </Typography>
                                                                                {selected && (
                                                                                    <Tooltip title="Selected">
                                                                                        <CheckCircleRounded color="primary" sx={{ fontSize: 18 }} />
                                                                                    </Tooltip>
                                                                                )}
                                                                            </Stack>
                                                                            <Typography variant="caption" sx={{
                                                                                color: "text.secondary"
                                                                            }}>
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

                                        <ScenarioScopeSelector
                                            title="Scenario Scope"
                                            description="Select the scenarios where this policy should apply."
                                            scenarioOptions={scenarioOptions}
                                            value={editorState.scenarios}
                                            onChange={(scenarios) => setEditorState((state) => ({ ...state, scenarios }))}
                                            helperText="Policies own their own scope. Groups only control organization and activation."
                                        />
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
                <Typography variant="body2" sx={{
                    color: "text.secondary"
                }}>
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
        </>
    );
};

export default PolicyEditorDialog;
