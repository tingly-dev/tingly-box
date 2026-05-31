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
import {
    Add as AddIcon,
    Delete as DeleteIcon,
    Close as CloseIcon,
    AutoAwesome as AutoAwesomeIcon,
    Article as ArticleIcon,
    Speed as SpeedIcon,
    Bolt as BoltIcon,
    HelpOutline as HelpOutlineIcon,
} from '@/components/icons';
import React, { useEffect, useMemo, useState } from 'react';
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
    tool_use: [
        { value: 'equals', label: 'Equals', description: 'Latest message is tool use and its name equals the value', valueType: 'string' },
    ],
    token: [
        { value: 'ge', label: '≥', description: 'Token count >= value', valueType: 'int' },
        { value: 'le', label: '≤', description: 'Token count <= value', valueType: 'int' },
    ],
    service_ttft: [
        { value: 'avg_le', label: 'Avg ≤', description: 'Best service avg TTFT <= value (ms)', valueType: 'int' },
        { value: 'avg_ge', label: 'Avg ≥', description: 'Mean avg TTFT across services >= value (ms)', valueType: 'int' },
        { value: 'max_le', label: 'P99 ≤', description: 'Best service P99 TTFT <= value (ms)', valueType: 'int' },
        { value: 'max_ge', label: 'P99 ≥', description: 'Mean P99 TTFT across services >= value (ms)', valueType: 'int' },
    ],
    service_capacity: [
        { value: 'util_le', label: 'Util ≤', description: 'Avg seat utilization across services <= value (%)', valueType: 'int' },
        { value: 'util_ge', label: 'Util ≥', description: 'Avg seat utilization across services >= value (%)', valueType: 'int' },
        { value: 'util_lt', label: 'Util <', description: 'Avg seat utilization across services < value (%)', valueType: 'int' },
        { value: 'util_gt', label: 'Util >', description: 'Avg seat utilization across services > value (%)', valueType: 'int' },
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

const CATEGORY_ORDER = ['agent', 'context', 'request', 'service'];

const positionMeta = (value: string): PositionMeta | undefined =>
    POSITION_OPTIONS.find((p) => p.value === value);

const categoryMeta = (cat: string): CategoryMeta =>
    CATEGORY_META[cat] || { label: cat, icon: <HelpOutlineIcon fontSize="small" /> };

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
    const [activeCategory, setActiveCategory] = useState<string>('agent');

    useEffect(() => {
        if (!open) return;
        const initialOps = smartRouting?.ops ? smartRouting.ops.map((o) => ({ ...o })) : [];
        setDescription(smartRouting?.description || '');
        setOps(initialOps);
        if (initialOps.length > 0) {
            const firstCat = positionMeta(initialOps[0].position)?.category || 'agent';
            setActiveCategory(firstCat);
        } else {
            setActiveCategory('agent');
        }
    }, [open, smartRouting]);

    const handleAddOpForPosition = (position: SmartOp['position']) => {
        const newOp: SmartOp = {
            uuid: uuidv4(),
            position,
            operation: '',
            value: '',
            meta: { description: '', type: 'string' },
        };
        const opOpts = OPERATION_OPTIONS[position as string];
        if (opOpts?.length === 1) {
            newOp.operation = opOpts[0].value;
            newOp.meta = { description: opOpts[0].description, type: opOpts[0].valueType };
        }
        setOps((prev) => [...prev, newOp]);
    };

    const handleRemoveOp = (opUuid: string) => {
        setOps((prev) => prev.filter((o) => o.uuid !== opUuid));
    };

    const updateOp = (opUuid: string, updates: Partial<SmartOp>) => {
        setOps((prev) =>
            prev.map((op) => {
                if (op.uuid !== opUuid) return op;
                const updated = { ...op, ...updates };
                if ('operation' in updates && updates.operation !== undefined) {
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
        const parsed = op.meta?.type === 'int' ? inputValue.replace(/,/g, '') : inputValue;
        updateOp(opUuid, { value: parsed });
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

    const countByCategory = useMemo(() => {
        const counts: Record<string, number> = {};
        ops.forEach((op) => {
            const cat = positionMeta(op.position)?.category || '';
            counts[cat] = (counts[cat] || 0) + 1;
        });
        return counts;
    }, [ops]);

    const invalidByCategory = useMemo(() => {
        const invalid: Record<string, boolean> = {};
        ops.forEach((op) => {
            if (!isOpValid(op)) {
                const cat = positionMeta(op.position)?.category || '';
                invalid[cat] = true;
            }
        });
        return invalid;
    }, [ops]);

    const currentCategoryPositions = useMemo(
        () => POSITION_OPTIONS.filter((p) => p.category === activeCategory),
        [activeCategory],
    );

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ pb: 1 }}>
                Smart Rule
                <Typography variant="caption" component="div" color="text.secondary">
                    Conditions evaluated with AND logic — all must match for the rule to trigger.
                </Typography>
            </DialogTitle>

            {/* Description strip */}
            <Box
                sx={{
                    px: 3,
                    py: 1.5,
                    borderTop: 1,
                    borderBottom: ops.length === 0 ? 1 : 0,
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

            {/* Active conditions strip — mirrors FlagCatalogDialog's active-flags strip */}
            {ops.length > 0 && (
                <Box
                    sx={{
                        px: 3,
                        py: 1.25,
                        borderTop: 1,
                        borderBottom: 1,
                        borderColor: 'divider',
                    }}
                >
                    <Stack direction="row" alignItems="center" spacing={1} flexWrap="wrap" useFlexGap>
                        <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                            Active ({ops.length})
                        </Typography>
                        {ops.map((op) => {
                            const cat = positionMeta(op.position)?.category || '';
                            const meta = categoryMeta(cat);
                            const valid = isOpValid(op);
                            return (
                                <Chip
                                    key={op.uuid}
                                    size="small"
                                    icon={meta.icon}
                                    label={opSummary(op)}
                                    onClick={() => setActiveCategory(cat || 'agent')}
                                    onDelete={() => handleRemoveOp(op.uuid)}
                                    deleteIcon={<CloseIcon />}
                                    color={valid ? 'default' : 'warning'}
                                    sx={{ maxWidth: 240 }}
                                />
                            );
                        })}
                    </Stack>
                </Box>
            )}

            <DialogContent sx={{ p: 0, display: 'flex', minHeight: 460 }} dividers={false}>
                {/* Left: category sidebar */}
                <Box
                    sx={{
                        width: 200,
                        flexShrink: 0,
                        borderRight: 1,
                        borderColor: 'divider',
                        bgcolor: 'background.paper',
                        overflowY: 'auto',
                    }}
                >
                    {CATEGORY_ORDER.map((cat) => {
                        const meta = categoryMeta(cat);
                        const count = countByCategory[cat] || 0;
                        const selected = cat === activeCategory;
                        return (
                            <Box
                                key={cat}
                                onClick={() => setActiveCategory(cat)}
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
                                <Box sx={{ color: selected ? 'primary.main' : 'text.secondary', display: 'flex' }}>
                                    {meta.icon}
                                </Box>
                                <Typography
                                    variant="body2"
                                    sx={{
                                        flexGrow: 1,
                                        fontWeight: selected ? 600 : 400,
                                        color: selected ? 'primary.main' : 'text.primary',
                                    }}
                                >
                                    {meta.label}
                                </Typography>
                                {count > 0 && (
                                    <Chip
                                        size="small"
                                        label={count}
                                        color={invalidByCategory[cat] ? 'warning' : 'primary'}
                                        variant="filled"
                                        sx={{ height: 18, fontSize: '0.65rem' }}
                                    />
                                )}
                            </Box>
                        );
                    })}
                </Box>

                {/* Right: position cards for active category */}
                <Box sx={{ flexGrow: 1, p: 2, overflowY: 'auto' }}>
                    <Stack spacing={1.5}>
                        {currentCategoryPositions.map((pos) => {
                            const posOps = ops.filter((op) => op.position === pos.value);
                            const opOpts = OPERATION_OPTIONS[pos.value as string] || [];

                            return (
                                <Box
                                    key={pos.value}
                                    sx={{
                                        p: 1.5,
                                        border: '1px solid',
                                        borderColor: posOps.length > 0 ? 'primary.light' : 'divider',
                                        borderRadius: 1,
                                        bgcolor: posOps.length > 0 ? 'action.hover' : 'transparent',
                                        transition: 'border-color 0.15s, background-color 0.15s',
                                    }}
                                >
                                    {/* Position header */}
                                    <Stack direction="row" alignItems="flex-start" spacing={1}>
                                        <Box sx={{ flexGrow: 1, minWidth: 0 }}>
                                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                {pos.label}
                                            </Typography>
                                            <Tooltip title={pos.description} placement="top-start" disableHoverListener={pos.description.length <= 60}>
                                                <Typography
                                                    variant="caption"
                                                    color="text.secondary"
                                                    sx={{
                                                        display: '-webkit-box',
                                                        WebkitLineClamp: 2,
                                                        WebkitBoxOrient: 'vertical',
                                                        overflow: 'hidden',
                                                    }}
                                                >
                                                    {pos.description}
                                                </Typography>
                                            </Tooltip>
                                        </Box>
                                        <Button
                                            size="small"
                                            startIcon={<AddIcon />}
                                            onClick={() => handleAddOpForPosition(pos.value)}
                                            variant={posOps.length > 0 ? 'text' : 'outlined'}
                                            sx={{ flexShrink: 0 }}
                                        >
                                            Add
                                        </Button>
                                    </Stack>

                                    {/* Inline condition rows */}
                                    {posOps.length > 0 && (
                                        <Stack spacing={1} sx={{ mt: 1.5 }}>
                                            {posOps.map((op) => {
                                                const valid = isOpValid(op);
                                                return (
                                                    <Box
                                                        key={op.uuid}
                                                        sx={{
                                                            p: 1.25,
                                                            bgcolor: 'background.paper',
                                                            borderRadius: 1,
                                                            border: '1px solid',
                                                            borderColor: valid ? 'divider' : 'warning.light',
                                                        }}
                                                    >
                                                        <Stack direction="row" alignItems="flex-start" spacing={1}>
                                                            <Box sx={{ flexGrow: 1 }}>
                                                                {/* Operation chips */}
                                                                {opOpts.length > 0 && (
                                                                    <Stack
                                                                        direction="row"
                                                                        spacing={0.5}
                                                                        flexWrap="wrap"
                                                                        useFlexGap
                                                                        sx={{ mb: op.meta?.type !== 'bool' && op.operation ? 1 : 0 }}
                                                                    >
                                                                        {opOpts.map((opt) => (
                                                                            <Tooltip
                                                                                key={opt.value}
                                                                                title={opt.description}
                                                                                placement="top"
                                                                            >
                                                                                <Chip
                                                                                    size="small"
                                                                                    label={opt.label}
                                                                                    onClick={() =>
                                                                                        updateOp(op.uuid, { operation: opt.value })
                                                                                    }
                                                                                    color={
                                                                                        op.operation === opt.value
                                                                                            ? 'primary'
                                                                                            : 'default'
                                                                                    }
                                                                                    variant={
                                                                                        op.operation === opt.value
                                                                                            ? 'filled'
                                                                                            : 'outlined'
                                                                                    }
                                                                                    sx={{ cursor: 'pointer' }}
                                                                                />
                                                                            </Tooltip>
                                                                        ))}
                                                                    </Stack>
                                                                )}

                                                                {/* Value input */}
                                                                {op.meta?.type !== 'bool' && op.operation && (
                                                                    VALUE_OPTIONS[op.position] ? (
                                                                        <FormControl size="small" sx={{ minWidth: 160 }}>
                                                                            <InputLabel>Value</InputLabel>
                                                                            <Select
                                                                                value={op.value || ''}
                                                                                label="Value"
                                                                                onChange={(e) =>
                                                                                    handleValueChange(op.uuid, e.target.value as string)
                                                                                }
                                                                            >
                                                                                <MenuItem value="">
                                                                                    <em>Select…</em>
                                                                                </MenuItem>
                                                                                {VALUE_OPTIONS[op.position]!.map((opt) => (
                                                                                    <MenuItem key={opt.value} value={opt.value}>
                                                                                        {opt.label}
                                                                                    </MenuItem>
                                                                                ))}
                                                                            </Select>
                                                                        </FormControl>
                                                                    ) : (
                                                                        <TextField
                                                                            size="small"
                                                                            label="Value"
                                                                            value={
                                                                                op.meta?.type === 'int'
                                                                                    ? formatNumberWithCommas(op.value || '')
                                                                                    : (op.value || '')
                                                                            }
                                                                            onChange={(e) =>
                                                                                handleValueChange(op.uuid, e.target.value)
                                                                            }
                                                                            placeholder={
                                                                                pos.value === 'service_capacity'
                                                                                    ? '0–100'
                                                                                    : pos.value === 'service_ttft'
                                                                                    ? 'ms'
                                                                                    : op.meta?.type === 'int'
                                                                                    ? '1,234'
                                                                                    : 'enter value'
                                                                            }
                                                                            sx={{ minWidth: 160 }}
                                                                        />
                                                                    )
                                                                )}
                                                            </Box>

                                                            <Tooltip title="Remove condition">
                                                                <IconButton
                                                                    size="small"
                                                                    onClick={() => handleRemoveOp(op.uuid)}
                                                                    sx={{
                                                                        flexShrink: 0,
                                                                        opacity: 0.4,
                                                                        '&:hover': { opacity: 1, color: 'error.main' },
                                                                    }}
                                                                >
                                                                    <DeleteIcon sx={{ fontSize: 16 }} />
                                                                </IconButton>
                                                            </Tooltip>
                                                        </Stack>
                                                    </Box>
                                                );
                                            })}
                                        </Stack>
                                    )}
                                </Box>
                            );
                        })}
                    </Stack>
                </Box>
            </DialogContent>

            <DialogActions>
                <Button onClick={onClose} color="primary">
                    Cancel
                </Button>
                <Button onClick={handleSave} color="primary" variant="contained" disabled={!allValid}>
                    Save
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default SmartRuleCatalogDialog;
