import {
    Add as AddIcon,
    Delete as DeleteIcon
} from '@mui/icons-material';
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
import React, { useEffect, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';
import type { SmartOp, SmartRouting } from './RoutingGraphTypes';

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

    // Reset form when smartRouting changes
    useEffect(() => {
        if (smartRouting) {
            setDescription(smartRouting.description || '');
            // Get existing ops or create one empty op
            const existingOps = smartRouting.ops && smartRouting.ops.length > 0
                ? [...smartRouting.ops]
                : [{
                    uuid: uuidv4(),
                    position: '' as SmartOp['position'],
                    operation: '',
                    value: '',
                    meta: {
                        description: '',
                        type: 'string',
                    },
                } as SmartOp];
            setOps(existingOps);
        } else {
            setDescription('');
            setOps([{
                uuid: uuidv4(),
                position: '' as SmartOp['position'],
                operation: '',
                value: '',
                meta: {
                    description: '',
                    type: 'string',
                },
            }]);
        }
    }, [smartRouting, open]);

    const handleSave = () => {
        if (!smartRouting) return;

        // Trim string values before saving and filter out empty ops
        const cleanedOps: SmartOp[] = ops
            .filter(op => op.position && op.operation) // Only keep valid ops
            .map(op => ({
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

    const handleAddOperation = () => {
        const newOp: SmartOp = {
            uuid: uuidv4(),
            position: '' as SmartOp['position'],
            operation: '',
            value: '',
            meta: {
                description: '',
                type: 'string',
            },
        };
        setOps([...ops, newOp]);
    };

    const handleRemoveOperation = (index: number) => {
        if (ops.length <= 1) return; // Keep at least one op
        setOps(ops.filter((_, i) => i !== index));
    };

    const handleOpFieldChangeAtIndex = (index: number, field: keyof SmartOp, value: any) => {
        const updatedOps = [...ops];
        const updatedOp = { ...updatedOps[index] };

        // When position changes, clear operation and value, reset metadata
        if (field === 'position') {
            updatedOp.operation = '';
            updatedOp.value = '';
            updatedOp.meta = {
                description: '',
                type: 'string',
            };
        }
        // Update operation-specific metadata when operation is set
        else if (field === 'operation') {
            const opDef = OPERATION_OPTIONS[updatedOp.position]?.find(opt => opt.value === value);
            if (opDef) {
                updatedOp.meta = {
                    description: opDef.description,
                    type: opDef.valueType,
                };
                // Clear value when operation changes
                updatedOp.value = '';
            }
        } else {
            updatedOp[field] = value;
        }

        updatedOps[index] = updatedOp;
        setOps(updatedOps);
    };

    const handleValueChangeAtIndex = (index: number, inputValue: string) => {
        const op = ops[index];
        const updatedOps = [...ops];

        if (op.meta?.type === 'int') {
            // For int type, store the raw number (without commas)
            updatedOps[index] = { ...op, value: parseNumberInput(inputValue) };
        } else {
            // For string type, store as-is
            updatedOps[index] = { ...op, value: inputValue };
        }
        setOps(updatedOps);
    };

    const isValid = () => {
        // At least one op must be complete
        return ops.some(op => {
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
            maxWidth="md"
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

                    {/* Operations Section */}
                    <Box>
                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                            <Typography variant="subtitle1" fontWeight={600}>
                                Operations (AND Logic)
                            </Typography>
                            <Button
                                startIcon={<AddIcon />}
                                onClick={handleAddOperation}
                                size="small"
                                variant="outlined"
                            >
                                Add Operation
                            </Button>
                        </Box>

                        {/* Operations List - each directly editable */}
                        <Stack spacing={2}>
                            {ops.map((op, index) => (
                                <Box
                                    key={op.uuid || index}
                                    sx={{
                                        p: 2,
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 1,
                                        bgcolor: 'background.paper',
                                        transition: 'all 0.2s',
                                        '&:hover': {
                                            borderColor: 'action.hover',
                                            backgroundColor: 'action.hover',
                                        },
                                    }}
                                >
                                    {/* Header with index and delete button */}
                                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                                        <Typography variant="subtitle2" color="text.secondary">
                                            Operation {index + 1}
                                        </Typography>
                                        {ops.length > 1 && (
                                            <IconButton
                                                size="small"
                                                onClick={() => handleRemoveOperation(index)}
                                                sx={{ color: 'error.main' }}
                                            >
                                                <DeleteIcon fontSize="small" />
                                            </IconButton>
                                        )}
                                    </Box>

                                    {/* Direct editor for this operation */}
                                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems={{ xs: 'stretch', sm: 'center' }}>
                                        {/* Position Select */}
                                        <FormControl size="small" sx={{ minWidth: 140 }}>
                                            <InputLabel>Position</InputLabel>
                                            <Select
                                                value={op.position || ''}
                                                label="Position"
                                                onChange={(e) => handleOpFieldChangeAtIndex(index, 'position', e.target.value)}
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
                                                onChange={(e) => handleOpFieldChangeAtIndex(index, 'operation', e.target.value)}
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
                                                value={op.meta?.type === 'int' ? formatNumberWithCommas(op.value || '') : (op.value || '')}
                                                onChange={(e) => handleValueChangeAtIndex(index, e.target.value)}
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
