import {
    Box,
    Button,
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
} from '@mui/material';
import React, { useEffect, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';
import type { SmartRouting, SmartOp } from './RoutingGraphTypes';

// Position options with descriptions
const POSITION_OPTIONS = [
    // { value: 'model', label: 'Model', description: 'Request model name' },
    { value: 'thinking', label: 'Thinking', description: 'Thinking mode enabled / disable' },
    { value: 'system', label: 'System Prompt', description: 'System prompt message' },
    { value: 'user', label: 'User Prompt', description: 'User prompt message' },
    { value: 'tool_use', label: 'Tool Name', description: 'Tool name' },
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
    system: [
        { value: 'any_contains', label: 'Any Contains', description: 'Any system messages contain the value', valueType: 'string' },
        // { value: 'regex', label: 'Regex', description: 'Any system messages match regex pattern', valueType: 'string' },
    ],
    user: [
        { value: 'any_contains', label: 'Any Contains', description: 'Any user messages contain the value', valueType: 'string' },
        { value: 'contains', label: 'Contains', description: 'Latest message is user role and it contains the value', valueType: 'string' },
        // { value: 'regex', label: 'Regex', description: 'Combined user messages match regex pattern', valueType: 'string' },
        // { value: 'type', label: 'Type', description: 'Latest message is user role and check its content type (e.g., image)', valueType: 'string' },
    ],
    tool_use: [
        { value: 'equals', label: 'Equals', description: 'Latest message is tool use and its name equals the value', valueType: 'string' },
    ],
    token: [
        { value: 'ge', label: 'Greater or Equal', description: 'Token count >= value', valueType: 'int' },
        { value: 'gt', label: 'Greater Than', description: 'Token count > value', valueType: 'int' },
        { value: 'le', label: 'Less or Equal', description: 'Token count <= value', valueType: 'int' },
        { value: 'lt', label: 'Less Than', description: 'Token count < value', valueType: 'int' },
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
    const [op, setOp] = useState<SmartOp>({
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
            // Get the first op if exists, otherwise create empty op
            const existingOp = smartRouting.ops && smartRouting.ops.length > 0
                ? { ...smartRouting.ops[0] }
                : {
                    uuid: uuidv4(),
                    position: '' as SmartOp['position'],
                    operation: '',
                    value: '',
                    meta: {
                        description: '',
                        type: 'string',
                    },
                } as SmartOp;
            setOp(existingOp);
        } else {
            setDescription('');
            setOp({
                uuid: uuidv4(),
                position: '' as SmartOp['position'],
                operation: '',
                value: '',
                meta: {
                    description: '',
                    type: 'string',
                },
            });
        }
    }, [smartRouting, open]);

    const handleSave = () => {
        if (!smartRouting) return;

        // Trim string value before saving
        const cleanedOp: SmartOp = {
            ...op,
            value: op.meta?.type === 'string' ? op.value?.trim() ?? '' : op.value,
        };

        const updated: SmartRouting = {
            ...smartRouting,
            description,
            ops: [cleanedOp],
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

    const handleOpFieldChange = (field: keyof SmartOp, value: any) => {
        const updatedOp = { ...op, [field]: value };

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
            const opDef = OPERATION_OPTIONS[op.position]?.find(opt => opt.value === value);
            if (opDef) {
                updatedOp.meta = {
                    description: opDef.description,
                    type: opDef.valueType,
                };
                // Clear value when operation changes
                updatedOp.value = '';
            }
        }

        setOp(updatedOp);
    };

    const handleValueChange = (inputValue: string) => {
        if (op.meta?.type === 'int') {
            // For int type, store the raw number (without commas)
            setOp({ ...op, value: parseNumberInput(inputValue) });
        } else {
            // For string type, store as-is
            setOp({ ...op, value: inputValue });
        }
    };

    const getDisplayValue = (): string => {
        if (op.meta?.type === 'int') {
            return formatNumberWithCommas(op.value || '');
        }
        return op.value || '';
    };

    const isValid = () => {
        if (!op.position || !op.operation) return false;
        if (op.meta?.type === 'bool') {
            return true; // bool operations don't require a value
        }
        return op.value && op.value.trim() !== '';
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

                    {/* Operation */}
                    <Box>
                        <Typography variant="subtitle1" fontWeight={600} sx={{ mb: 2 }}>
                            Operation
                        </Typography>

                        <Box
                            sx={{
                                p: 2,
                                border: '1px solid',
                                borderColor: 'divider',
                                borderRadius: 1,
                                bgcolor: 'background.paper',
                            }}
                        >
                            <Stack direction="row" spacing={2} alignItems="center">
                                {/* Position Select */}
                                <FormControl size="small" sx={{ minWidth: 120 }}>
                                    <InputLabel>Position</InputLabel>
                                    <Select
                                        value={op.position || ''}
                                        label="Position"
                                        onChange={(e) => handleOpFieldChange('position', e.target.value)}
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
                                        onChange={(e) => handleOpFieldChange('operation', e.target.value)}
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
                                        value={getDisplayValue()}
                                        onChange={(e) => handleValueChange(e.target.value)}
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
