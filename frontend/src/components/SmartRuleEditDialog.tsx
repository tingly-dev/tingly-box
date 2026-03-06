import {
    Box,
    Button,
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
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import React, { useEffect, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';
import type { SmartRouting, SmartOp } from './RoutingGraphTypes';

// Position options with descriptions
const POSITION_OPTIONS = [
    // { value: 'model', label: 'Model', description: 'Request model name' },
    { value: 'context_system', label: 'System Prompt', description: 'System prompt message in context' },
    // { value: 'context_user', label: 'User Context', description: 'User messages in context' },
    { value: 'latest_user', label: 'Latest User Message', description: 'Latest user message' },
    // { value: 'tool_use', label: 'Tool Name', description: 'Tool name' },
    { value: 'thinking', label: 'Thinking', description: 'Thinking mode enabled / disable' },
    { value: 'token', label: 'Token Count', description: 'Token count' },
] as const;

// Operation options grouped by position
const OPERATION_OPTIONS: Record<string, Array<{ value: string; label: string; description: string; valueType: 'string' | 'int' | 'bool' }>> = {
    model: [
        { value: 'contains', label: 'Contains', description: 'Model name contains the value', valueType: 'string' },
        // { value: 'glob', label: 'Glob', description: 'Model name matches glob pattern', valueType: 'string' },
        { value: 'equals', label: 'Equals', description: 'Model name equals the value', valueType: 'string' },
    ],
    thinking: [
        { value: 'enabled', label: 'Enabled', description: 'Thinking mode is enabled', valueType: 'bool' },
        { value: 'disabled', label: 'Disabled', description: 'Thinking mode is disabled', valueType: 'bool' },
    ],
    context_system: [
        { value: 'contains', label: 'Contains', description: 'Any system messages contain the value', valueType: 'string' },
        // { value: 'regex', label: 'Regex', description: 'Any system messages match regex pattern', valueType: 'string' },
    ],
    context_user: [
        { value: 'contains', label: 'Contains', description: 'Any user messages contain the value', valueType: 'string' },
        // { value: 'regex', label: 'Regex', description: 'Combined user messages match regex pattern', valueType: 'string' },
    ],
    latest_user: [
        { value: 'contains', label: 'Contains', description: 'Latest user message contains the value', valueType: 'string' },
        // { value: 'type', label: 'Type', description: 'Latest user message content type (e.g., image)', valueType: 'string' },
    ],
    tool_use: [
        { value: 'equals', label: 'Equals', description: 'Latest message is tool use and its name equals the value', valueType: 'string' },
    ],
    token: [
        { value: 'ge', label: 'Greater or Equal', description: 'Token count >= value', valueType: 'int' },
        // { value: 'gt', label: 'Greater Than', description: 'Token count > value', valueType: 'int' },
        { value: 'le', label: 'Less or Equal', description: 'Token count <= value', valueType: 'int' },
        // { value: 'lt', label: 'Less Than', description: 'Token count < value', valueType: 'int' },
    ],
};

export interface SmartRuleEditDialogProps {
    open: boolean;
    smartRouting: SmartRouting | null;
    onSave: (updated: SmartRouting) => void;
    onCancel: () => void;
}

const SmartRuleEditDialog: React.FC<SmartRuleEditDialogProps> = ({
    open,
    smartRouting,
    onSave,
    onCancel,
}) => {
    const [description, setDescription] = useState('');
    const [ops, setOps] = useState<SmartOp[]>([]);

    // Create empty operation template
    const createEmptyOp = (): SmartOp => ({
        uuid: uuidv4(),
        position: '' as SmartOp['position'],
        operation: '',
        value: '',
        meta: {
            description: '',
            type: 'string',
        },
    });

    // Reset form when smartRouting changes
    useEffect(() => {
        if (smartRouting) {
            setDescription(smartRouting.description || '');
            // Use existing ops or create one empty op
            setOps(smartRouting.ops && smartRouting.ops.length > 0
                ? [...smartRouting.ops]
                : [createEmptyOp()]
            );
        } else {
            setDescription('');
            setOps([createEmptyOp()]);
        }
    }, [smartRouting, open]);

    const handleSave = () => {
        if (!smartRouting) return;

        // Trim string values before saving
        const cleanedOps: SmartOp[] = ops.map(op => ({
            ...op,
            value: op.meta?.type === 'string' ? op.value?.trim() ?? '' : op.value,
        }));

        const updated: SmartRouting = {
            ...smartRouting,
            description,
            ops: cleanedOps,
        };
        onSave(updated);
    };

    const addOperation = () => {
        setOps([...ops, createEmptyOp()]);
    };

    const removeOperation = (uuid: string) => {
        if (ops.length <= 1) return; // Keep at least one operation
        setOps(ops.filter(op => op.uuid !== uuid));
    };

    const updateOperation = (uuid: string, updates: Partial<SmartOp>) => {
        setOps(ops.map(op => {
            if (op.uuid !== uuid) return op;

            const updatedOp = { ...op, ...updates };

            // When position changes, clear operation and value, reset metadata
            if ('position' in updates && updates.position !== undefined) {
                updatedOp.operation = '';
                updatedOp.value = '';
                updatedOp.meta = {
                    description: '',
                    type: 'string',
                };
            }
            // Update operation-specific metadata when operation is set
            else if ('operation' in updates && updates.operation !== undefined) {
                const opDef = OPERATION_OPTIONS[op.position]?.find(opt => opt.value === updates.operation);
                if (opDef) {
                    updatedOp.meta = {
                        description: opDef.description,
                        type: opDef.valueType,
                    };
                    // Clear value when operation changes
                    updatedOp.value = '';
                }
            }

            return updatedOp;
        }));
    };

    // Format number with thousand separators for display
    const formatNumberWithCommas = (value: string): string => {
        if (!value) return '';
        const numStr = value.replace(/,/g, '');
        if (!/^\d*$/.test(numStr)) return value;
        const num = parseInt(numStr, 10);
        if (isNaN(num)) return '';
        return num.toLocaleString('en-US');
    };

    // Parse number input (remove commas) for storage
    const parseNumberInput = (value: string): string => {
        return value.replace(/,/g, '');
    };

    const getDisplayValue = (op: SmartOp): string => {
        if (op.meta?.type === 'int') {
            return formatNumberWithCommas(op.value || '');
        }
        return op.value || '';
    };

    const handleValueChange = (uuid: string, inputValue: string) => {
        const op = ops.find(o => o.uuid === uuid);
        if (!op) return;

        let parsedValue: string;
        if (op.meta?.type === 'int') {
            parsedValue = parseNumberInput(inputValue);
        } else {
            parsedValue = inputValue;
        }

        updateOperation(uuid, { value: parsedValue });
    };

    const isValid = () => {
        return ops.every(op => {
            if (!op.position || !op.operation) return false;
            if (op.meta?.type === 'bool') {
                return true; // bool operations don't require a value
            }
            return op.value && op.value.trim() !== '';
        });
    };

    return (
        <Dialog
            open={open}
            onClose={onCancel}
            maxWidth="sm"
            fullWidth
        >
            <DialogTitle>Edit Smart Rule</DialogTitle>
            <DialogContent>
                <Stack spacing={3} sx={{ mt: 1 }}>
                    {/* Description */}
                    <TextField
                        label="Description"
                        fullWidth
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                        placeholder="e.g., Route image requests to vision model"
                    />

                    {/* Operations */}
                    <Box>
                        <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
                            <Typography variant="subtitle1" fontWeight={600}>
                                Operations (AND Logic)
                            </Typography>
                            <Box sx={{ flex: 1 }} />
                            <Button
                                startIcon={<AddIcon />}
                                onClick={addOperation}
                                variant="outlined"
                                size="small"
                            >
                                Add Condition
                            </Button>
                        </Stack>

                        {/* Operations List */}
                        <Stack spacing={2}>
                            {ops.map((op, index) => (
                                <Box
                                    key={op.uuid}
                                    sx={{
                                        p: 2,
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 1,
                                        bgcolor: 'background.paper',
                                    }}
                                >
                                    {/* Operation Header */}
                                    <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 1.5 }}>
                                        <Typography variant="caption" color="text.secondary">
                                            Condition {index + 1}
                                        </Typography>
                                        <Box sx={{ flex: 1 }} />
                                        {ops.length > 1 && (
                                            <Tooltip title="Remove this condition">
                                                <IconButton
                                                    size="small"
                                                    color="error"
                                                    onClick={() => removeOperation(op.uuid)}
                                                >
                                                    <DeleteIcon fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                    </Stack>

                                    <Stack direction="row" spacing={2} alignItems="center">
                                        {/* Position Select */}
                                        <FormControl size="small" sx={{ minWidth: 120 }}>
                                            <InputLabel>Position</InputLabel>
                                            <Select
                                                value={op.position || ''}
                                                label="Position"
                                                onChange={(e) => updateOperation(op.uuid, { position: e.target.value as SmartOp['position'] })}
                                            >
                                                <MenuItem value="">
                                                    <em>Select...</em>
                                                </MenuItem>
                                                {POSITION_OPTIONS.map((opt) => (
                                                    <MenuItem key={opt.value} value={opt.value}>
                                                        <Tooltip title={opt.description} placement="right">
                                                            <span style={{ width: '100%' }}>{opt.label}</span>
                                                        </Tooltip>
                                                    </MenuItem>
                                                ))}
                                            </Select>
                                        </FormControl>

                                        {/* Operation Select */}
                                        <FormControl size="small" sx={{ minWidth: 150 }}>
                                            <InputLabel>Operation</InputLabel>
                                            <Select
                                                value={op.operation || ''}
                                                label="Operation"
                                                onChange={(e) => updateOperation(op.uuid, { operation: e.target.value })}
                                                disabled={!op.position}
                                            >
                                                <MenuItem value="">
                                                    <em>Select...</em>
                                                </MenuItem>
                                                {OPERATION_OPTIONS[op.position]?.map((opt) => (
                                                    <MenuItem key={opt.value} value={opt.value}>
                                                        <Tooltip title={opt.description} placement="right">
                                                            <span style={{ width: '100%' }}>{opt.label}</span>
                                                        </Tooltip>
                                                    </MenuItem>
                                                ))}
                                            </Select>
                                        </FormControl>

                                        {/* Value Input - only show for string and int types */}
                                        {op.meta?.type !== 'bool' && (
                                            <TextField
                                                size="small"
                                                label="Value"
                                                value={getDisplayValue(op)}
                                                onChange={(e) => handleValueChange(op.uuid, e.target.value)}
                                                placeholder={
                                                    op.meta?.type === 'int' ? '1,234' :
                                                    'enter value'
                                                }
                                                sx={{ flex: 1 }}
                                                type="text"
                                            />
                                        )}
                                    </Stack>

                                    {/* Operation Description */}
                                    {op.meta?.description && (
                                        <Typography
                                            variant="caption"
                                            color="text.secondary"
                                            sx={{ mt: 1, display: 'block' }}
                                        >
                                            {op.meta.description}
                                        </Typography>
                                    )}
                                </Box>
                            ))}
                        </Stack>
                    </Box>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                <Button onClick={onCancel} color="inherit">
                    Cancel
                </Button>
                <Button
                    onClick={handleSave}
                    variant="contained"
                    disabled={!isValid()}
                >
                    Save
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default SmartRuleEditDialog;
