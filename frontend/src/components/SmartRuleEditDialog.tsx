import {
    Add as AddIcon,
    Delete as DeleteIcon,
    DragIndicator as DragIcon,
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
    Typography,
} from '@mui/material';
import React, { useEffect, useState } from 'react';
import { v4 as uuidv4 } from 'uuid';
import type { SmartRouting, SmartOp } from './RoutingGraphTypes';

// Position options with descriptions
const POSITION_OPTIONS = [
    { value: 'model', label: 'Model', description: 'Request model name' },
    { value: 'thinking', label: 'Thinking', description: 'Thinking mode enabled' },
    { value: 'system', label: 'System', description: 'System message content' },
    { value: 'user', label: 'User', description: 'User message content' },
    { value: 'tool_use', label: 'Tool Use', description: 'Tool use/name' },
    { value: 'token', label: 'Token', description: 'Token count' },
] as const;

// Operation options grouped by position
const OPERATION_OPTIONS: Record<string, Array<{ value: string; label: string; description: string; valueType: 'string' | 'int' | 'bool' }>> = {
    model: [
        { value: 'contains', label: 'Contains', description: 'Model name contains the value', valueType: 'string' },
        { value: 'glob', label: 'Glob', description: 'Model name matches glob pattern', valueType: 'string' },
        { value: 'equals', label: 'Equals', description: 'Model name equals the value', valueType: 'string' },
    ],
    thinking: [
        { value: 'enabled', label: 'Enabled', description: 'Thinking mode is enabled', valueType: 'bool' },
        { value: 'disabled', label: 'Disabled', description: 'Thinking mode is disabled', valueType: 'bool' },
    ],
    system: [
        { value: 'any_contains', label: 'Any Contains', description: 'Any system messages contain the value', valueType: 'string' },
        { value: 'regex', label: 'Regex', description: 'Any system messages match regex pattern', valueType: 'string' },
    ],
    user: [
        { value: 'any_contains', label: 'Any Contains', description: 'Any user messages contain the value', valueType: 'string' },
        { value: 'contains', label: 'Contains', description: 'Latest message is user role and it contains the value', valueType: 'string' },
        { value: 'regex', label: 'Regex', description: 'Combined user messages match regex pattern', valueType: 'string' },
        { value: 'type', label: 'Type', description: 'Latest message is user role and check its content type (e.g., image)', valueType: 'string' },
    ],
    tool_use: [
        { value: 'is', label: 'Is', description: 'Latest message is tool use and its name is the value', valueType: 'string' },
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
    const [ops, setOps] = useState<SmartOp[]>([]);

    // Reset form when smartRouting changes
    useEffect(() => {
        if (smartRouting) {
            console.log('SmartRuleEditDialog: Loading rule:', smartRouting.uuid, smartRouting.description);
            console.log('SmartRuleEditDialog: ops:', smartRouting.ops);
            setDescription(smartRouting.description || '');
            // Deep copy ops to avoid mutating the original
            setOps(JSON.parse(JSON.stringify(smartRouting.ops || [])));
        } else {
            setDescription('');
            setOps([]);
        }
    }, [smartRouting, open]);

    const handleSave = () => {
        if (!smartRouting) return;

        const updated: SmartRouting = {
            ...smartRouting,
            description,
            ops,
        };

        console.log('SmartRuleEditDialog: Saving rule:', updated.uuid, updated.description);
        console.log('SmartRuleEditDialog: ops after edit:', updated.ops);

        onSave(updated);
    };

    const handleAddOp = () => {
        const newOp: SmartOp = {
            uuid: uuidv4(),
            position: 'model',
            operation: 'contains',
            value: '',
            meta: {
                description: '',
                type: 'string',
            },
        };
        setOps([...ops, newOp]);
    };

    const handleRemoveOp = (index: number) => {
        setOps(ops.filter((_, i) => i !== index));
    };

    const handleOpChange = (index: number, field: keyof SmartOp, value: any) => {
        const updatedOps = [...ops];
        updatedOps[index] = { ...updatedOps[index], [field]: value };

        // Update operation-specific metadata
        if (field === 'position' || field === 'operation') {
            const position = updatedOps[index].position;
            const operation = updatedOps[index].operation;
            const opDef = OPERATION_OPTIONS[position]?.find(op => op.value === operation);
            if (opDef) {
                updatedOps[index].meta = {
                    description: opDef.description,
                    type: opDef.valueType,
                };
            }
        }

        setOps(updatedOps);
    };

    const isValid = ops.length > 0 && ops.every(op => {
        if (op.meta?.type === 'bool') {
            return true; // bool operations don't require a value
        }
        return op.value && op.value.trim() !== '';
    });

    return (
        <Dialog
            open={open}
            onClose={onCancel}
            maxWidth="md"
            fullWidth
            PaperProps={{
                sx: { height: '80vh' }
            }}
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

                    {/* Operations List */}
                    <Box>
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                            <Typography variant="subtitle1" fontWeight={600}>
                                Operations
                            </Typography>
                            <Button
                                startIcon={<AddIcon />}
                                onClick={handleAddOp}
                                size="small"
                                variant="outlined"
                            >
                                Add Operation
                            </Button>
                        </Box>

                        {ops.length === 0 ? (
                            <Box
                                sx={{
                                    py: 4,
                                    textAlign: 'center',
                                    border: '1px dashed',
                                    borderColor: 'divider',
                                    borderRadius: 1,
                                }}
                            >
                                <Typography variant="body2" color="text.secondary">
                                    No operations yet. Click "Add Operation" to create one.
                                </Typography>
                            </Box>
                        ) : (
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
                                        }}
                                    >
                                        <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1 }}>
                                            <DragIcon sx={{ mt: 1, color: 'text.secondary' }} />
                                            <Box sx={{ flex: 1 }}>
                                                <Stack direction="row" spacing={2} alignItems="center">
                                                    {/* Position Select */}
                                                    <FormControl size="small" sx={{ minWidth: 120 }}>
                                                        <InputLabel>Position</InputLabel>
                                                        <Select
                                                            value={op.position}
                                                            label="Position"
                                                            onChange={(e) => handleOpChange(index, 'position', e.target.value)}
                                                        >
                                                            {POSITION_OPTIONS.map((opt) => (
                                                                <MenuItem key={opt.value} value={opt.value}>
                                                                    {opt.label}
                                                                </MenuItem>
                                                            ))}
                                                        </Select>
                                                    </FormControl>

                                                    {/* Operation Select */}
                                                    <FormControl size="small" sx={{ minWidth: 150 }}>
                                                        <InputLabel>Operation</InputLabel>
                                                        <Select
                                                            value={op.operation}
                                                            label="Operation"
                                                            onChange={(e) => handleOpChange(index, 'operation', e.target.value)}
                                                            disabled={!op.position}
                                                        >
                                                            {OPERATION_OPTIONS[op.position]?.map((opt) => (
                                                                <MenuItem key={opt.value} value={opt.value}>
                                                                    {opt.label}
                                                                </MenuItem>
                                                            ))}
                                                        </Select>
                                                    </FormControl>

                                                    {/* Value Input */}
                                                    <TextField
                                                        size="small"
                                                        label="Value"
                                                        value={op.value || ''}
                                                        onChange={(e) => handleOpChange(index, 'value', e.target.value)}
                                                        placeholder={
                                                            op.meta?.type === 'int' ? '123' :
                                                            op.meta?.type === 'bool' ? 'auto-detected' :
                                                            'enter value'
                                                        }
                                                        disabled={op.meta?.type === 'bool'}
                                                        sx={{ flex: 1 }}
                                                        type={op.meta?.type === 'int' ? 'number' : 'text'}
                                                    />

                                                    {/* Delete Button */}
                                                    <IconButton
                                                        size="small"
                                                        color="error"
                                                        onClick={() => handleRemoveOp(index)}
                                                    >
                                                        <DeleteIcon />
                                                    </IconButton>
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
                                    </Box>
                                ))}
                            </Stack>
                        )}
                    </Box>
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button onClick={onCancel}>Cancel</Button>
                <Button
                    onClick={handleSave}
                    variant="contained"
                    disabled={!isValid}
                >
                    Save
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default SmartRuleEditDialog;
