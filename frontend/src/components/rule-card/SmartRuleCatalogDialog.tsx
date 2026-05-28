import {
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Tooltip,
    Typography,
    IconButton,
} from '@mui/material';
import {
    Add as AddIcon,
    Delete as DeleteIcon,
    AutoAwesome as AutoAwesomeIcon,
    Article as ArticleIcon,
    Speed as SpeedIcon,
    Bolt as BoltIcon,
    HelpOutline as HelpOutlineIcon,
} from '@/components/icons';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';
import type { SmartOp, SmartRouting } from '@/components/RoutingGraphTypes';

interface PositionMeta {
    value: SmartOp['position'];
    label: string;
    description: string;
    category: string;
}

const POSITION_OPTIONS: PositionMeta[] = [
    { value: 'agent.claude_code', label: 'Agent: Claude Code', description: 'Claude Code request kind (main / subagent / compact)', category: 'agent' },
    { value: 'context_system', label: 'System Prompt', description: 'System prompt message in context', category: 'context' },
    { value: 'latest_user', label: 'Latest User Message', description: 'Latest user message', category: 'context' },
    { value: 'thinking', label: 'Thinking', description: 'Thinking mode enabled / disable', category: 'request' },
    { value: 'token', label: 'Token Count', description: 'Token count', category: 'request' },
    { value: 'service_ttft', label: 'Service TTFT', description: 'Time to first token across services (ms)', category: 'service' },
    { value: 'service_capacity', label: 'Service Capacity', description: 'Seat utilization across services (%)', category: 'service' },
    { value: 'proxy_vision', label: 'Proxy Vision', description: "Vision-proxy bypass: matched rule's services describe images, request continues to downstream with image blocks replaced by text", category: 'request' },
];

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

interface CategoryMeta {
    label: string;
    icon: React.ReactElement;
}

const CATEGORY_META: Record<string, CategoryMeta> = {
    agent: { label: 'Agent', icon: <AutoAwesomeIcon fontSize="small" /> },
    context: { label: 'Context', icon: <ArticleIcon fontSize="small" /> },
    request: { label: 'Request', icon: <BoltIcon fontSize="small" /> },
    service: { label: 'Service', icon: <SpeedIcon fontSize="small" /> },
};

const positionMeta = (value: string): PositionMeta | undefined =>
    POSITION_OPTIONS.find((p) => p.value === value);

const categoryMeta = (cat: string): CategoryMeta => CATEGORY_META[cat] || { label: cat, icon: <HelpOutlineIcon fontSize="small" /> };

const createEmptyOp = (): SmartOp => ({
    uuid: uuidv4(),
    position: '' as SmartOp['position'],
    operation: '',
    value: '',
    meta: { description: '', type: 'string' },
});

const formatNumberWithCommas = (value: string): string => {
    if (!value) return '';
    const numStr = value.replace(/,/g, '');
    if (!/^\d*$/.test(numStr)) return value;
    const num = parseInt(numStr, 10);
    if (isNaN(num)) return '';
    return num.toLocaleString('en-US');
};

const isOpValid = (op: SmartOp): boolean => {
    if (!op.position || !op.operation) return false;
    if (op.meta?.type === 'bool') return true;
    return !!op.value && op.value.trim() !== '';
};

const opSummary = (op: SmartOp): string => {
    if (!op.position) return 'Unset';
    const pos = positionMeta(op.position);
    const posLabel = pos?.label || op.position;
    if (!op.operation) return posLabel;
    if (op.meta?.type === 'bool') return `${posLabel} · ${op.operation}`;
    return `${posLabel} · ${op.operation}${op.value ? ` · ${op.value}` : ''}`;
};

export interface SmartRuleCatalogDialogProps {
    open: boolean;
    smartRouting: SmartRouting | null;
    onClose: () => void;
    onSave: (updated: SmartRouting) => void;
}

export const SmartRuleCatalogDialog: React.FC<SmartRuleCatalogDialogProps> = ({
    open,
    smartRouting,
    onClose,
    onSave,
}) => {
    const [description, setDescription] = useState('');
    const [ops, setOps] = useState<SmartOp[]>([]);
    const [selectedOpId, setSelectedOpId] = useState<string | undefined>();
    const opRefs = useRef<Record<string, HTMLDivElement | null>>({});

    useEffect(() => {
        if (!open) return;
        const initialOps = smartRouting?.ops && smartRouting.ops.length > 0
            ? smartRouting.ops.map((o) => ({ ...o }))
            : [createEmptyOp()];
        setDescription(smartRouting?.description || '');
        setOps(initialOps);
        setSelectedOpId(initialOps[0]?.uuid);
    }, [open, smartRouting]);

    const selectedOp = useMemo(() => ops.find((o) => o.uuid === selectedOpId), [ops, selectedOpId]);

    const handleAddOp = () => {
        const newOp = createEmptyOp();
        setOps((prev) => [...prev, newOp]);
        setSelectedOpId(newOp.uuid);
    };

    const handleRemoveOp = (opUuid: string) => {
        setOps((prev) => {
            if (prev.length <= 1) return prev;
            const next = prev.filter((o) => o.uuid !== opUuid);
            if (selectedOpId === opUuid) {
                setSelectedOpId(next[0]?.uuid);
            }
            return next;
        });
    };

    const updateOp = (opUuid: string, updates: Partial<SmartOp>) => {
        setOps((prev) =>
            prev.map((op) => {
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
        );
    };

    const handleValueChange = (opUuid: string, inputValue: string) => {
        const op = ops.find((o) => o.uuid === opUuid);
        if (!op) return;
        const parsedValue = op.meta?.type === 'int' ? inputValue.replace(/,/g, '') : inputValue;
        updateOp(opUuid, { value: parsedValue });
    };

    const handleSave = () => {
        if (!smartRouting) return;
        const cleanedOps: SmartOp[] = ops.map((op) => ({
            ...op,
            value: op.meta?.type === 'string' ? (op.value?.trim() ?? '') : op.value,
        }));
        onSave({ ...smartRouting, description, ops: cleanedOps });
    };

    const allValid = ops.every(isOpValid);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ pb: 1 }}>
                Smart Rule
                <Typography variant="caption" component="div" color="text.secondary">
                    Conditions evaluated with AND logic — all must match for the rule to trigger.
                </Typography>
            </DialogTitle>

            {/* Description strip — mirrors the active-flags strip on FlagCatalogDialog. */}
            <Box
                sx={{
                    px: 3,
                    py: 1.5,
                    borderTop: 1,
                    borderBottom: 1,
                    borderColor: 'divider',
                    bgcolor: 'action.hover',
                }}
            >
                <TextField
                    label="Description"
                    fullWidth
                    size="small"
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    placeholder="e.g., Route image requests to vision model"
                />
            </Box>

            <DialogContent sx={{ p: 0, display: 'flex', minHeight: 420 }} dividers={false}>
                {/* Left: conditions sidebar */}
                <Box
                    sx={{
                        width: 240,
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
                        <Button startIcon={<AddIcon />} onClick={handleAddOp} size="small" variant="outlined" fullWidth>
                            Add Condition
                        </Button>
                    </Box>
                    {ops.map((op, index) => {
                        const selected = op.uuid === selectedOpId;
                        const valid = isOpValid(op);
                        const meta = op.position ? categoryMeta(positionMeta(op.position)?.category || '') : { label: '', icon: <HelpOutlineIcon fontSize="small" /> };
                        return (
                            <Box
                                key={op.uuid}
                                ref={(el: HTMLDivElement | null) => { opRefs.current[op.uuid] = el; }}
                                onClick={() => setSelectedOpId(op.uuid)}
                                sx={{
                                    px: 2,
                                    py: 1.25,
                                    cursor: 'pointer',
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: 1,
                                    borderLeft: 3,
                                    borderLeftColor: selected ? 'primary.main' : 'transparent',
                                    bgcolor: selected ? 'action.selected' : 'transparent',
                                    '&:hover': {
                                        bgcolor: selected ? 'action.selected' : 'action.hover',
                                    },
                                    transition: 'background-color 0.15s',
                                }}
                            >
                                <Box sx={{ color: selected ? 'primary.main' : 'text.secondary', display: 'flex', flexShrink: 0 }}>
                                    {meta.icon}
                                </Box>
                                <Box sx={{ flexGrow: 1, minWidth: 0 }}>
                                    <Typography
                                        variant="body2"
                                        sx={{
                                            fontWeight: selected ? 600 : 500,
                                            color: selected ? 'primary.main' : 'text.primary',
                                            fontSize: '0.8rem',
                                            lineHeight: 1.2,
                                        }}
                                    >
                                        Condition {index + 1}
                                    </Typography>
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            display: 'block',
                                            color: 'text.secondary',
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis',
                                            whiteSpace: 'nowrap',
                                            fontSize: '0.7rem',
                                        }}
                                    >
                                        {opSummary(op)}
                                    </Typography>
                                </Box>
                                {!valid && (
                                    <Chip size="small" label="!" color="warning" sx={{ height: 16, fontSize: '0.6rem', flexShrink: 0 }} />
                                )}
                                {ops.length > 1 && (
                                    <Tooltip title="Remove condition">
                                        <IconButton
                                            size="small"
                                            onClick={(e) => { e.stopPropagation(); handleRemoveOp(op.uuid); }}
                                            sx={{ p: 0.25, flexShrink: 0, opacity: 0.5, '&:hover': { opacity: 1, color: 'error.main' } }}
                                        >
                                            <DeleteIcon sx={{ fontSize: 14 }} />
                                        </IconButton>
                                    </Tooltip>
                                )}
                            </Box>
                        );
                    })}
                </Box>

                {/* Right: condition editor pane */}
                <Box sx={{ flexGrow: 1, p: 3, overflowY: 'auto' }}>
                    {!selectedOp ? (
                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'text.disabled' }}>
                            <Typography variant="body2">Select a condition to configure it.</Typography>
                        </Box>
                    ) : (
                        <Stack spacing={2}>
                            <Box>
                                <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.25 }}>
                                    Configure condition
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                    Pick what to inspect (Position), how to match it (Operation), and the comparison value.
                                </Typography>
                            </Box>

                            <FormControl size="small" fullWidth>
                                <InputLabel>Position</InputLabel>
                                <Select
                                    value={selectedOp.position || ''}
                                    label="Position"
                                    onChange={(e) => updateOp(selectedOp.uuid, { position: e.target.value as SmartOp['position'] })}
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

                            <FormControl size="small" fullWidth disabled={!selectedOp.position}>
                                <InputLabel>Operation</InputLabel>
                                <Select
                                    value={selectedOp.operation || ''}
                                    label="Operation"
                                    onChange={(e) => updateOp(selectedOp.uuid, { operation: e.target.value })}
                                >
                                    <MenuItem value=""><em>Select…</em></MenuItem>
                                    {OPERATION_OPTIONS[selectedOp.position]?.map((opt) => (
                                        <MenuItem key={opt.value} value={opt.value}>
                                            <Tooltip title={opt.description} placement="right">
                                                <span style={{ width: '100%' }}>{opt.label}</span>
                                            </Tooltip>
                                        </MenuItem>
                                    ))}
                                </Select>
                            </FormControl>

                            {selectedOp.meta?.type !== 'bool' && selectedOp.operation && (
                                VALUE_OPTIONS[selectedOp.position] ? (
                                    <FormControl size="small" fullWidth>
                                        <InputLabel>Value</InputLabel>
                                        <Select
                                            value={selectedOp.value || ''}
                                            label="Value"
                                            onChange={(e) => handleValueChange(selectedOp.uuid, e.target.value as string)}
                                        >
                                            <MenuItem value=""><em>Select…</em></MenuItem>
                                            {VALUE_OPTIONS[selectedOp.position]!.map((opt) => (
                                                <MenuItem key={opt.value} value={opt.value}>{opt.label}</MenuItem>
                                            ))}
                                        </Select>
                                    </FormControl>
                                ) : (
                                    <TextField
                                        size="small"
                                        label="Value"
                                        fullWidth
                                        value={selectedOp.meta?.type === 'int' ? formatNumberWithCommas(selectedOp.value || '') : (selectedOp.value || '')}
                                        onChange={(e) => handleValueChange(selectedOp.uuid, e.target.value)}
                                        placeholder={
                                            selectedOp.position === 'service_capacity' ? '0–100' :
                                            selectedOp.position === 'service_ttft' ? 'ms' :
                                            selectedOp.meta?.type === 'int' ? '1,234' :
                                            'enter value'
                                        }
                                    />
                                )
                            )}

                            {selectedOp.meta?.description && (
                                <Typography variant="caption" color="text.secondary">
                                    {selectedOp.meta.description}
                                </Typography>
                            )}
                        </Stack>
                    )}
                </Box>
            </DialogContent>

            <DialogActions>
                <Button onClick={onClose} color="primary">Cancel</Button>
                <Button onClick={handleSave} color="primary" variant="contained" disabled={!allValid}>
                    Save
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default SmartRuleCatalogDialog;
