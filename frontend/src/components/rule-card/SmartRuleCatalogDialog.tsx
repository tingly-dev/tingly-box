import {
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    IconButton,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { Add as AddIcon, Delete as DeleteIcon, Rule as RuleIcon } from '@/components/icons';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';
import type { SmartOp, SmartRouting } from '@/components/RoutingGraphTypes';

// Position options with descriptions
const POSITION_OPTIONS = [
    { value: 'agent.claude_code', label: 'Agent: Claude Code', description: 'Claude Code request kind (main / subagent / compact)' },
    { value: 'context_system', label: 'System Prompt', description: 'System prompt message in context' },
    { value: 'latest_user', label: 'Latest User Message', description: 'Latest user message' },
    { value: 'thinking', label: 'Thinking', description: 'Thinking mode enabled / disable' },
    { value: 'token', label: 'Token Count', description: 'Token count' },
    { value: 'service_ttft', label: 'Service TTFT', description: 'Time to first token across services (ms)' },
    { value: 'service_capacity', label: 'Service Capacity', description: 'Seat utilization across services (%)' },
    { value: 'proxy_vision', label: 'Proxy Vision', description: "Vision-proxy bypass: matched rule's services describe images, request continues to downstream with image blocks replaced by text" },
] as const;

const VALUE_OPTIONS: Record<string, Array<{ value: string; label: string }> | undefined> = {
    'agent.claude_code': [
        { value: 'main', label: 'Main' },
        { value: 'subagent', label: 'Subagent' },
        { value: 'compact', label: 'Compact' },
    ],
};

const OPERATION_OPTIONS: Record<string, Array<{ value: string; label: string; description: string; valueType: 'string' | 'int' | 'bool' }>> = {
    model: [
        { value: 'contains', label: 'Contains', description: 'Model name contains the value', valueType: 'string' },
        { value: 'equals', label: 'Equals', description: 'Model name equals the value', valueType: 'string' },
    ],
    thinking: [
        { value: 'enabled', label: 'Enabled', description: 'Thinking mode is enabled', valueType: 'bool' },
        { value: 'disabled', label: 'Disabled', description: 'Thinking mode is disabled', valueType: 'bool' },
    ],
    context_system: [
        { value: 'contains', label: 'Contains', description: 'Any system messages contain the value', valueType: 'string' },
    ],
    context_user: [
        { value: 'contains', label: 'Contains', description: 'Any user messages contain the value', valueType: 'string' },
    ],
    latest_user: [
        { value: 'contains', label: 'Contains', description: 'Latest user message contains the value', valueType: 'string' },
    ],
    proxy_vision: [
        { value: 'enabled', label: 'Enabled', description: "Vision-proxy bypass is enabled — image-bearing requests are described by the matched rule's services and forwarded as text to the downstream model", valueType: 'bool' },
    ],
    tool_use: [
        { value: 'equals', label: 'Equals', description: 'Latest message is tool use and its name equals the value', valueType: 'string' },
    ],
    token: [
        { value: 'ge', label: 'Greater or Equal', description: 'Token count >= value', valueType: 'int' },
        { value: 'le', label: 'Less or Equal', description: 'Token count <= value', valueType: 'int' },
    ],
    service_ttft: [
        { value: 'avg_le', label: 'Avg ≤', description: 'Best service avg TTFT <= value (ms)', valueType: 'int' },
        { value: 'avg_ge', label: 'Avg ≥', description: 'Mean avg TTFT across services >= value (ms)', valueType: 'int' },
        { value: 'max_le', label: 'P99 ≤', description: 'Best service P99 TTFT <= value (ms)', valueType: 'int' },
        { value: 'max_ge', label: 'P99 ≥', description: 'Mean P99 TTFT across services >= value (ms)', valueType: 'int' },
    ],
    service_capacity: [
        { value: 'util_le', label: 'Utilization ≤', description: 'Avg seat utilization across services <= value (%)', valueType: 'int' },
        { value: 'util_ge', label: 'Utilization ≥', description: 'Avg seat utilization across services >= value (%)', valueType: 'int' },
        { value: 'util_lt', label: 'Utilization <', description: 'Avg seat utilization across services < value (%)', valueType: 'int' },
        { value: 'util_gt', label: 'Utilization >', description: 'Avg seat utilization across services > value (%)', valueType: 'int' },
    ],
    'agent.claude_code': [
        { value: 'equals', label: 'Equals', description: 'Claude Code request kind equals the value', valueType: 'string' },
    ],
};

const createEmptyOp = (): SmartOp => ({
    uuid: uuidv4(),
    position: '' as SmartOp['position'],
    operation: '',
    value: '',
    meta: { description: '', type: 'string' },
});

const createEmptyRule = (): SmartRouting => ({
    uuid: uuidv4(),
    description: '',
    ops: [createEmptyOp()],
    services: [],
});

const formatNumberWithCommas = (value: string): string => {
    if (!value) return '';
    const numStr = value.replace(/,/g, '');
    if (!/^\d*$/.test(numStr)) return value;
    const num = parseInt(numStr, 10);
    if (isNaN(num)) return '';
    return num.toLocaleString('en-US');
};

const isRuleValid = (rule: SmartRouting): boolean =>
    rule.ops.every((op) => {
        if (!op.position || !op.operation) return false;
        if (op.meta?.type === 'bool') return true;
        return op.value && op.value.trim() !== '';
    });

export interface SmartRuleCatalogDialogProps {
    open: boolean;
    smartRouting: SmartRouting[];
    initialRuleId?: string;
    onClose: () => void;
    onSave: (updated: SmartRouting[]) => void;
}

export const SmartRuleCatalogDialog: React.FC<SmartRuleCatalogDialogProps> = ({
    open,
    smartRouting,
    initialRuleId,
    onClose,
    onSave,
}) => {
    const [draft, setDraft] = useState<SmartRouting[]>([]);
    const [selectedRuleId, setSelectedRuleId] = useState<string | undefined>();
    const selectedPaneRef = useRef<HTMLDivElement | null>(null);

    useEffect(() => {
        if (!open) return;
        const copy = smartRouting.map((r) => ({ ...r, ops: r.ops.map((o) => ({ ...o })) }));
        setDraft(copy);
        setSelectedRuleId(initialRuleId ?? (copy.length > 0 ? copy[0].uuid : undefined));
    }, [open, smartRouting, initialRuleId]);

    const selectedRule = useMemo(() => draft.find((r) => r.uuid === selectedRuleId), [draft, selectedRuleId]);

    const updateSelectedRule = (updater: (r: SmartRouting) => SmartRouting) => {
        setDraft((d) => d.map((r) => (r.uuid === selectedRuleId ? updater(r) : r)));
    };

    const handleAddRule = () => {
        const newRule = createEmptyRule();
        setDraft((d) => [...d, newRule]);
        setSelectedRuleId(newRule.uuid);
    };

    const handleDeleteRule = (ruleUuid: string) => {
        setDraft((d) => {
            const next = d.filter((r) => r.uuid !== ruleUuid);
            if (selectedRuleId === ruleUuid) {
                setSelectedRuleId(next.length > 0 ? next[0].uuid : undefined);
            }
            return next;
        });
    };

    const handleAddOp = () => {
        updateSelectedRule((r) => ({ ...r, ops: [...r.ops, createEmptyOp()] }));
    };

    const handleRemoveOp = (opUuid: string) => {
        updateSelectedRule((r) => {
            if (r.ops.length <= 1) return r;
            return { ...r, ops: r.ops.filter((o) => o.uuid !== opUuid) };
        });
    };

    const handleUpdateOp = (opUuid: string, updates: Partial<SmartOp>) => {
        updateSelectedRule((r) => ({
            ...r,
            ops: r.ops.map((op) => {
                if (op.uuid !== opUuid) return op;
                const updated = { ...op, ...updates };
                if ('position' in updates && updates.position !== undefined) {
                    updated.operation = '';
                    updated.value = '';
                    updated.meta = { description: '', type: 'string' };
                    const opts = OPERATION_OPTIONS[updates.position as string];
                    if (opts && opts.length === 1) {
                        updated.operation = opts[0].value;
                        updated.meta = { description: opts[0].description, type: opts[0].valueType };
                    }
                } else if ('operation' in updates && updates.operation !== undefined) {
                    const opDef = OPERATION_OPTIONS[op.position]?.find((o) => o.value === updates.operation);
                    if (opDef) {
                        updated.meta = { description: opDef.description, type: opDef.valueType };
                        updated.value = '';
                    }
                }
                return updated;
            }),
        }));
    };

    const handleValueChange = (opUuid: string, inputValue: string) => {
        const op = selectedRule?.ops.find((o) => o.uuid === opUuid);
        if (!op) return;
        const parsedValue = op.meta?.type === 'int' ? inputValue.replace(/,/g, '') : inputValue;
        handleUpdateOp(opUuid, { value: parsedValue });
    };

    const allValid = draft.every(isRuleValid);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ pb: 1 }}>
                Smart Rules
                <Typography variant="caption" component="div" color="text.secondary">
                    Configure smart routing rules applied at the rule level.
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ p: 0, display: 'flex', minHeight: 460 }} dividers={false}>
                {/* Left: rules sidebar */}
                <Box
                    sx={{
                        width: 220,
                        flexShrink: 0,
                        borderRight: 1,
                        borderColor: 'divider',
                        bgcolor: 'background.paper',
                        display: 'flex',
                        flexDirection: 'column',
                        overflowY: 'auto',
                    }}
                >
                    <Box sx={{ p: 1.5, borderBottom: 1, borderColor: 'divider' }}>
                        <Button
                            startIcon={<AddIcon />}
                            onClick={handleAddRule}
                            size="small"
                            variant="outlined"
                            fullWidth
                        >
                            Add Rule
                        </Button>
                    </Box>

                    {draft.length === 0 ? (
                        <Box sx={{ p: 2, color: 'text.disabled', fontSize: '0.8rem', textAlign: 'center', mt: 2 }}>
                            No smart rules yet.
                        </Box>
                    ) : (
                        draft.map((rule, index) => {
                            const selected = rule.uuid === selectedRuleId;
                            const valid = isRuleValid(rule);
                            return (
                                <Box
                                    key={rule.uuid}
                                    onClick={() => setSelectedRuleId(rule.uuid)}
                                    sx={{
                                        px: 2,
                                        py: 1.25,
                                        cursor: 'pointer',
                                        display: 'flex',
                                        alignItems: 'center',
                                        gap: 0.75,
                                        borderLeft: 3,
                                        borderLeftColor: selected ? 'primary.main' : 'transparent',
                                        bgcolor: selected ? 'action.selected' : 'transparent',
                                        '&:hover': {
                                            bgcolor: selected ? 'action.selected' : 'action.hover',
                                        },
                                        transition: 'background-color 0.15s',
                                    }}
                                >
                                    <RuleIcon sx={{ fontSize: 16, color: selected ? 'primary.main' : 'text.secondary', flexShrink: 0 }} />
                                    <Box sx={{ flexGrow: 1, minWidth: 0 }}>
                                        <Typography
                                            variant="body2"
                                            sx={{
                                                fontWeight: selected ? 600 : 400,
                                                color: selected ? 'primary.main' : 'text.primary',
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                                fontSize: '0.8rem',
                                            }}
                                        >
                                            {rule.description || `Rule ${index + 1}`}
                                        </Typography>
                                    </Box>
                                    <Chip
                                        size="small"
                                        label={rule.ops.length}
                                        color={valid ? (selected ? 'primary' : 'default') : 'warning'}
                                        variant={selected ? 'filled' : 'outlined'}
                                        sx={{ height: 18, fontSize: '0.65rem', flexShrink: 0 }}
                                    />
                                    <Tooltip title="Delete rule">
                                        <IconButton
                                            size="small"
                                            onClick={(e) => { e.stopPropagation(); handleDeleteRule(rule.uuid); }}
                                            sx={{ p: 0.25, flexShrink: 0, opacity: 0.5, '&:hover': { opacity: 1, color: 'error.main' } }}
                                        >
                                            <DeleteIcon sx={{ fontSize: 14 }} />
                                        </IconButton>
                                    </Tooltip>
                                </Box>
                            );
                        })
                    )}
                </Box>

                {/* Right: rule editor pane */}
                <Box ref={selectedPaneRef} sx={{ flexGrow: 1, p: 2, overflowY: 'auto' }}>
                    {!selectedRule ? (
                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'text.disabled' }}>
                            <Typography variant="body2">Select a rule to configure it, or add a new one.</Typography>
                        </Box>
                    ) : (
                        <Stack spacing={2.5}>
                            <TextField
                                label="Description"
                                fullWidth
                                size="small"
                                value={selectedRule.description}
                                onChange={(e) => updateSelectedRule((r) => ({ ...r, description: e.target.value }))}
                                placeholder="e.g., Route image requests to vision model"
                            />

                            <Box>
                                <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1.5 }}>
                                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                        Conditions{' '}
                                        <Typography component="span" variant="caption" color="text.secondary">
                                            (AND logic)
                                        </Typography>
                                    </Typography>
                                    <Button startIcon={<AddIcon />} onClick={handleAddOp} variant="outlined" size="small">
                                        Add Condition
                                    </Button>
                                </Stack>

                                <Stack spacing={1.5}>
                                    {selectedRule.ops.map((op, index) => (
                                        <Box
                                            key={op.uuid}
                                            sx={{
                                                p: 1.5,
                                                border: '1px solid',
                                                borderColor: op.position && op.operation ? 'primary.light' : 'divider',
                                                borderRadius: 1,
                                                bgcolor: op.position && op.operation ? 'action.hover' : 'transparent',
                                                transition: 'border-color 0.15s, background-color 0.15s',
                                            }}
                                        >
                                            <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
                                                <Typography variant="caption" color="text.secondary">
                                                    Condition {index + 1}
                                                </Typography>
                                                <Box sx={{ flex: 1 }} />
                                                {selectedRule.ops.length > 1 && (
                                                    <Tooltip title="Remove condition">
                                                        <IconButton
                                                            size="small"
                                                            color="error"
                                                            onClick={() => handleRemoveOp(op.uuid)}
                                                            sx={{ p: 0.5 }}
                                                        >
                                                            <DeleteIcon sx={{ fontSize: 16 }} />
                                                        </IconButton>
                                                    </Tooltip>
                                                )}
                                            </Stack>

                                            <Stack direction="row" spacing={1.5} alignItems="center" flexWrap="wrap" useFlexGap>
                                                <FormControl size="small" sx={{ minWidth: 160 }}>
                                                    <InputLabel>Position</InputLabel>
                                                    <Select
                                                        value={op.position || ''}
                                                        label="Position"
                                                        onChange={(e) => handleUpdateOp(op.uuid, { position: e.target.value as SmartOp['position'] })}
                                                    >
                                                        <MenuItem value=""><em>Select…</em></MenuItem>
                                                        {POSITION_OPTIONS.map((opt) => (
                                                            <MenuItem key={opt.value} value={opt.value}>
                                                                <Tooltip title={opt.description} placement="right">
                                                                    <span style={{ width: '100%' }}>{opt.label}</span>
                                                                </Tooltip>
                                                            </MenuItem>
                                                        ))}
                                                    </Select>
                                                </FormControl>

                                                <FormControl size="small" sx={{ minWidth: 150 }}>
                                                    <InputLabel>Operation</InputLabel>
                                                    <Select
                                                        value={op.operation || ''}
                                                        label="Operation"
                                                        disabled={!op.position}
                                                        onChange={(e) => handleUpdateOp(op.uuid, { operation: e.target.value })}
                                                    >
                                                        <MenuItem value=""><em>Select…</em></MenuItem>
                                                        {OPERATION_OPTIONS[op.position]?.map((opt) => (
                                                            <MenuItem key={opt.value} value={opt.value}>
                                                                <Tooltip title={opt.description} placement="right">
                                                                    <span style={{ width: '100%' }}>{opt.label}</span>
                                                                </Tooltip>
                                                            </MenuItem>
                                                        ))}
                                                    </Select>
                                                </FormControl>

                                                {op.meta?.type !== 'bool' && (
                                                    VALUE_OPTIONS[op.position] ? (
                                                        <FormControl size="small" sx={{ flex: 1, minWidth: 130 }}>
                                                            <InputLabel>Value</InputLabel>
                                                            <Select
                                                                value={op.value || ''}
                                                                label="Value"
                                                                onChange={(e) => handleValueChange(op.uuid, e.target.value as string)}
                                                            >
                                                                <MenuItem value=""><em>Select…</em></MenuItem>
                                                                {VALUE_OPTIONS[op.position]!.map((opt) => (
                                                                    <MenuItem key={opt.value} value={opt.value}>{opt.label}</MenuItem>
                                                                ))}
                                                            </Select>
                                                        </FormControl>
                                                    ) : (
                                                        <TextField
                                                            size="small"
                                                            label="Value"
                                                            value={op.meta?.type === 'int' ? formatNumberWithCommas(op.value || '') : (op.value || '')}
                                                            onChange={(e) => handleValueChange(op.uuid, e.target.value)}
                                                            placeholder={
                                                                op.position === 'service_capacity' ? '0–100' :
                                                                op.position === 'service_ttft' ? 'ms' :
                                                                op.meta?.type === 'int' ? '1,234' :
                                                                'enter value'
                                                            }
                                                            sx={{ flex: 1, minWidth: 120 }}
                                                        />
                                                    )
                                                )}
                                            </Stack>

                                            {op.meta?.description && (
                                                <Typography variant="caption" color="text.secondary" sx={{ mt: 0.75, display: 'block' }}>
                                                    {op.meta.description}
                                                </Typography>
                                            )}
                                        </Box>
                                    ))}
                                </Stack>
                            </Box>
                        </Stack>
                    )}
                </Box>
            </DialogContent>

            <DialogActions>
                <Button onClick={onClose} color="primary">Cancel</Button>
                <Button onClick={() => onSave(draft)} color="primary" variant="contained" disabled={!allValid}>
                    Save
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default SmartRuleCatalogDialog;
