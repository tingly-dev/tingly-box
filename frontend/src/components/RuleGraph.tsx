import React, { useState } from 'react';
import {
    Delete as DeleteIcon,
    ExpandLess as ExpandLessIcon,
    ExpandMore as ExpandMoreIcon,
    Save as SaveIcon,
    MoreVert as MoreVertIcon,
    Settings as SettingsIcon,
    Refresh as RefreshIcon,
    ArrowForward as ArrowForwardIcon,
    Edit as EditIcon,
    CheckCircle as CheckCircleIcon,
    RadioButtonUnchecked as RadioButtonUncheckedIcon,
    Add as AddIcon
} from '@mui/icons-material';
import {
    Box,
    Button,
    Card,
    CardContent,
    Chip,
    Collapse,
    IconButton,
    Stack,
    Switch,
    TextField,
    Typography,
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText,
    Tooltip
} from '@mui/material';
import { styled } from '@mui/material/styles';
import ProviderConfig from './ProviderConfig';

interface ConfigProvider {
    uuid: string;
    provider: string;
    model: string;
    isManualInput?: boolean;
    weight?: number;
    active?: boolean;
    time_window?: number;
}

interface ConfigRecord {
    uuid: string;
    requestModel: string;
    responseModel: string;
    active: boolean;
    providers: ConfigProvider[];
}

interface RuleGraphProps {
    record: ConfigRecord;
    providers: any[];
    providerModels: any;
    providerUuidToName: { [uuid: string]: string };
    saving: boolean;
    expanded: boolean;
    recordUuid: string;  // Add recordUuid prop
    onUpdateRecord: (field: keyof ConfigRecord, value: any) => void;
    onUpdateProvider: (recordId: string, providerId: string, field: keyof ConfigProvider, value: any) => void;
    onAddProvider: () => void;
    onDeleteProvider: (recordId: string, providerId: string) => void;
    onRefreshModels: (providerUuid: string) => void;
    onSave: () => void;
    onDelete: () => void;
    onReset: () => void;
    onToggleExpanded: () => void;
}

const StyledCard = styled(Card, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active }) => ({
    transition: 'all 0.2s ease-in-out',
}));

const SummarySection = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: theme.spacing(2),
    cursor: 'pointer',
    '&:hover': {
        backgroundColor: 'action.hover',
    },
}));

// Graph Container for expanded view
const GraphContainer = styled(Box)(({ theme }) => ({
    padding: theme.spacing(3),
    backgroundColor: 'grey.50',
    borderRadius: theme.shape.borderRadius,
    margin: theme.spacing(2),
}));

const GraphRow = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: theme.spacing(3),
    marginBottom: theme.spacing(2),
}));

const NodeContainer = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: theme.spacing(1),
}));

const ProviderNode = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    padding: theme.spacing(2.5),
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    width: 180,  // Fixed width - same as model nodes
    height: 200,  // Fixed height - same as model nodes
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    '&:hover': {
        borderColor: 'primary.main',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    }
}));

const ConnectionLine = styled(Box)(({ }) => ({
    display: 'flex',
    alignItems: 'center',
    color: 'text.secondary',
    fontSize: '1.5rem',
    '& svg': {
        fontSize: '2rem',
    }
}));

// Enhanced Model Node with editing support
const ModelNode: React.FC<{
    active: boolean;
    label: string;
    value: string;
    editable?: boolean;
    onUpdate?: (value: string) => void;
    showStatusIcon?: boolean;
}> = ({ active, label, value, editable = false, onUpdate, showStatusIcon = true }) => {
    const [editMode, setEditMode] = useState(false);
    const [tempValue, setTempValue] = useState(value);

    React.useEffect(() => {
        setTempValue(value);
    }, [value]);

    const handleSave = () => {
        if (onUpdate && tempValue.trim()) {
            onUpdate(tempValue.trim());
        }
        setEditMode(false);
    };

    const handleCancel = () => {
        setTempValue(value);
        setEditMode(false);
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter') {
            handleSave();
        } else if (e.key === 'Escape') {
            handleCancel();
        }
    };

    return (
        <Box sx={{ position: 'relative', display: 'flex', alignItems: 'center', gap: 1 }}>
            <StyledModelNode>
                {editMode && editable ? (
                    <TextField
                        value={tempValue}
                        onChange={(e) => setTempValue(e.target.value)}
                        onBlur={handleSave}
                        onKeyDown={handleKeyDown}
                        size="small"
                        fullWidth
                        autoFocus
                        sx={{
                            '& .MuiInputBase-input': {
                                color: 'text.primary',
                                fontWeight: 'inherit',
                                fontSize: 'inherit',
                                textAlign: 'center',
                                backgroundColor: 'transparent',
                            },
                            '& .MuiOutlinedInput-notchedOutline': {
                                borderColor: 'primary.main',
                            },
                            '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': {
                                borderColor: 'primary.dark',
                            },
                        }}
                    />
                ) : (
                    <Box
                        onClick={() => editable && setEditMode(true)}
                        sx={{
                            cursor: editable ? 'pointer' : 'default',
                            width: '100%',
                            py: 1.5,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            gap: 1,
                            '&:hover': editable ? {
                                '&::after': {
                                    content: '""',
                                    position: 'absolute',
                                    bottom: -4,
                                    left: '50%',
                                    transform: 'translateX(-50%)',
                                    width: 30,
                                    height: 2,
                                    backgroundColor: 'primary.main',
                                    borderRadius: 1,
                                }
                            } : {}
                        }}
                    >
                        {showStatusIcon && (
                            active ? (
                                <CheckCircleIcon sx={{ fontSize: 16, color: 'success.main' }} />
                            ) : (
                                <RadioButtonUncheckedIcon sx={{ fontSize: 16, color: 'text.disabled' }} />
                            )
                        )}
                        <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary', fontSize: '0.9rem' }}>
                            {value || label}
                        </Typography>
                    </Box>
                )}
            </StyledModelNode>
        </Box>
    );
};

// Styled model node with unified fixed size
const StyledModelNode = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: theme.spacing(2.5),
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    textAlign: 'center',
    width: 180,  // Fixed width
    height: 200,  // Fixed height - same as provider nodes
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    cursor: 'pointer',
    '&:hover': {
        borderColor: 'primary.main',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-1px)',
    }
}));

// Provider Node Component for Graph View
const ProviderNodeComponent: React.FC<{
    provider: ConfigProvider;
    apiStyle: string;
    availableProviders: any[];
    onUpdate: (field: keyof ConfigProvider, value: any) => void;
    providerModels: any;
    active: boolean;
    onDelete: () => void;
    onRefreshModels: () => void;
    providerUuidToName: { [uuid: string]: string };
}> = ({ provider, apiStyle, availableProviders, onUpdate, providerModels, active, onDelete, onRefreshModels, providerUuidToName }) => {
    const [editMode, setEditMode] = React.useState({
        provider: false,
        model: false
    });
    const [anchorEl, setAnchorEl] = React.useState<null | HTMLElement>(null);
    const menuOpen = Boolean(anchorEl);

    const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        setAnchorEl(null);
    };

    const handleRefresh = () => {
        handleMenuClose();
        onRefreshModels();
    };

    const handleDelete = () => {
        handleMenuClose();
        onDelete();
    };

    return (
        <ProviderNode>
            {/* API Style Chip */}
            {provider.provider && (
                <Chip
                    label={apiStyle}
                    size="small"
                    variant="outlined"
                    sx={{ fontSize: '0.7rem', height: 20, mb: 1.5 }}
                />
            )}

            {/* Provider Select */}
            <Box sx={{ width: '100%', mb: 1.5 }}>
                {editMode.provider ? (
                    <TextField
                        select
                        label="Provider"
                        value={provider.provider} // This is UUID
                        onChange={(e) => {
                            onUpdate('provider', e.target.value); // Store UUID
                            setEditMode({ ...editMode, provider: false });
                        }}
                        onBlur={() => setEditMode({ ...editMode, provider: false })}
                        size="small"
                        fullWidth
                        autoFocus
                        sx={{
                            '& .MuiInputLabel-root': {
                                fontSize: '0.75rem',
                            },
                            '& .MuiSelect-select': {
                                fontSize: '0.8rem',
                            }
                        }}
                    >
                        {availableProviders.map((p) => (
                            <MenuItem key={p.uuid} value={p.uuid}> {/* Use UUID as value */}
                                {p.name} {/* Display provider name */}
                            </MenuItem>
                        ))}
                    </TextField>
                ) : (
                    <Box
                        onClick={() => active && setEditMode({ ...editMode, provider: true })}
                        sx={{
                            cursor: active ? 'pointer' : 'default',
                            '&:hover': active ? { backgroundColor: 'action.hover', borderRadius: 1 } : {},
                            p: 1,
                            transition: 'background-color 0.2s',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            gap: 0.5,
                            width: '100%'
                        }}
                    >
                        {active ? (
                            <CheckCircleIcon sx={{ fontSize: 14, color: 'success.main' }} />
                        ) : (
                            <RadioButtonUncheckedIcon sx={{ fontSize: 14, color: 'text.disabled' }} />
                        )}
                        <Typography
                            variant="body2"
                            sx={{
                                fontWeight: 600,
                                color: 'text.primary',
                                textAlign: 'center',
                                fontSize: '0.9rem'
                            }}
                        >
                            {providerUuidToName[provider.provider] || 'Select provider'} {/* Convert UUID to name for display */}
                        </Typography>
                    </Box>
                )}
            </Box>

            {/* Model Select */}
            {provider.provider && (
                <Box sx={{ width: '100%', mb: 1.5 }}>
                    {editMode.model ? (
                        <TextField
                            select
                            label="Model"
                            value={provider.model}
                            onChange={(e) => {
                                onUpdate('model', e.target.value);
                                setEditMode({ ...editMode, model: false });
                            }}
                            onBlur={() => setEditMode({ ...editMode, model: false })}
                            size="small"
                            fullWidth
                            autoFocus
                        >
                            {(providerModels[providerUuidToName[provider.provider]]?.models || []).map((model: string) => (
                                <MenuItem key={model} value={model}>
                                    {model}
                                </MenuItem>
                            ))}
                        </TextField>
                    ) : (
                        <Box
                            onClick={() => active && setEditMode({ ...editMode, model: true })}
                            sx={{
                                cursor: active ? 'pointer' : 'default',
                                '&:hover': active ? { backgroundColor: 'action.hover', borderRadius: 1 } : {},
                                p: 1,
                                transition: 'background-color 0.2s',
                                textAlign: 'center'
                            }}
                        >
                            <Typography
                                variant="body2"
                                sx={{
                                    color: 'text.secondary',
                                    fontSize: '0.8rem',
                                    fontStyle: !provider.model ? 'italic' : 'normal'
                                }}
                            >
                                {provider.model || 'Select model'}
                            </Typography>
                        </Box>
                    )}
                </Box>
            )}

            {/*/!* Weight indicator *!/*/}
            {/*{provider.weight && (*/}
            {/*    <Chip*/}
            {/*        label={`weight: ${provider.weight}`}*/}
            {/*        size="small"*/}
            {/*        sx={{ fontSize: '0.7rem', height: 20, backgroundColor: 'grey.100', mt: 'auto' }}*/}
            {/*    />*/}
            {/*)}*/}

            {/* Action Menu Button */}
            <IconButton
                size="small"
                onClick={handleMenuClick}
                sx={{
                    position: 'absolute',
                    bottom: 4,
                    right: 4,
                    p: 0.5,
                    color: 'text.secondary',
                    '&:hover': {
                        backgroundColor: 'action.hover',
                        color: 'text.primary',
                    }
                }}
            >
                <MoreVertIcon fontSize="small" />
            </IconButton>

            {/* Action Menu */}
            <Menu
                anchorEl={anchorEl}
                open={menuOpen}
                onClose={handleMenuClose}
                onClick={(e) => e.stopPropagation()}
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
            >
                <MenuItem onClick={handleRefresh} disabled={!provider.provider || !active}>
                    <ListItemIcon>
                        <RefreshIcon fontSize="small" />
                    </ListItemIcon>
                    <ListItemText>Refresh Models</ListItemText>
                </MenuItem>
                <MenuItem onClick={() => {
                    handleMenuClose();
                    // Edit functionality - could expand provider or focus on specific field
                    setEditMode({ provider: true, model: false });
                }} disabled={!active}>
                    <ListItemIcon>
                        <EditIcon fontSize="small" />
                    </ListItemIcon>
                    <ListItemText>Edit Provider</ListItemText>
                </MenuItem>
                <MenuItem onClick={handleDelete} disabled={!active}>
                    <ListItemIcon>
                        <DeleteIcon fontSize="small" color="error" />
                    </ListItemIcon>
                    <ListItemText sx={{ color: 'error.main' }}>Delete Provider</ListItemText>
                </MenuItem>
            </Menu>
        </ProviderNode>
    );
};

// Main RuleGraph Component
const RuleGraph: React.FC<RuleGraphProps> = ({
    record,
    providers,
    providerModels,
    providerUuidToName,
    saving,
    expanded,
    recordUuid,
    onUpdateRecord,
    onUpdateProvider,
    onAddProvider,
    onDeleteProvider,
    onRefreshModels,
    onSave,
    onDelete,
    onReset,
    onToggleExpanded
}) => {
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [showResponseField, setShowResponseField] = useState(false);
    const menuOpen = Boolean(menuAnchorEl);

    const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setMenuAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        setMenuAnchorEl(null);
    };

    const handleConfigureResponse = () => {
        handleMenuClose();
        console.log('Configure response for:', record.responseModel);
    };

    const handleConfigureResponseModel = () => {
        handleMenuClose();
        setShowResponseField(true);
        if (!expanded) {
            onToggleExpanded();
        }
        setTimeout(() => {
            const responseField = document.getElementById(`response-model-${record.uuid}`) as HTMLInputElement;
            if (responseField) {
                responseField.focus();
                responseField.select();
            }
        }, 100);
    };

    const getApiStyle = (providerUuid: string) => {
        const provider = providers.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    };

    return (
        <StyledCard active={record.active}>
            {/* Header Section - RuleCard Style */}
            <SummarySection onClick={onToggleExpanded}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1 }}>
                    <Typography variant="h6" sx={{ fontWeight: 600, color: 'text.primary' }}>
                        {record.requestModel || 'Specified model name'}
                    </Typography>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Chip
                            label={`use ${record.providers.length} keys`}
                            size="small"
                            variant="outlined"
                        />
                        <Chip
                            label={record.active ? 'Active' : 'Inactive'}
                            color={record.active ? 'success' : 'default'}
                            size="small"
                        />
                        {record.responseModel && <Chip
                            label={`Response as ${record.responseModel}`}
                            size="small"
                            color={'info'}
                        />}
                    </Box>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    {/* Action Buttons */}
                    <Box sx={{ display: 'flex', gap: 0.5, mr: 1 }}>
                        <Button
                            variant="contained"
                            color="primary"
                            size="small"
                            startIcon={<SaveIcon />}
                            onClick={(e) => {
                                e.stopPropagation();
                                onSave();
                            }}
                            disabled={saving}
                            sx={{ minWidth: 'auto', px: 1.5 }}
                        >
                            {saving ? 'Saving...' : 'Save'}
                        </Button>
                        <Button
                            startIcon={<RefreshIcon />}
                            onClick={(e) => {
                                e.stopPropagation();
                                onReset();
                            }}
                            variant="outlined"
                            size="small"
                            disabled={saving}
                            sx={{ minWidth: 'auto', px: 1.5 }}
                        >
                            Reset
                        </Button>
                        <IconButton
                            size="small"
                            onClick={handleMenuClick}
                            disabled={saving}
                            sx={{ ml: 0.5 }}
                        >
                            <MoreVertIcon />
                        </IconButton>
                        <Menu
                            anchorEl={menuAnchorEl}
                            open={menuOpen}
                            onClose={handleMenuClose}
                            onClick={(e) => e.stopPropagation()}
                        >
                            {!record.responseModel ? (
                                <MenuItem onClick={handleConfigureResponseModel}>
                                    <ListItemIcon>
                                        <SettingsIcon fontSize="small" />
                                    </ListItemIcon>
                                    <ListItemText>Configure Response Model</ListItemText>
                                </MenuItem>
                            ) : (
                                <MenuItem onClick={handleConfigureResponse}>
                                    <ListItemIcon>
                                        <SettingsIcon fontSize="small" />
                                    </ListItemIcon>
                                    <ListItemText>Configure Response</ListItemText>
                                </MenuItem>
                            )}
                            <MenuItem onClick={(e) => {
                                e.stopPropagation();
                                onDelete();
                            }}>
                                <ListItemIcon>
                                    <DeleteIcon fontSize="small" />
                                </ListItemIcon>
                                <ListItemText>Delete Rule</ListItemText>
                            </MenuItem>
                        </Menu>
                    </Box>
                    <Switch
                        checked={record.active}
                        onChange={(e) => onUpdateRecord('active', e.target.checked)}
                        size="small"
                        disabled={saving}
                        onClick={(e) => e.stopPropagation()}
                    />
                    <IconButton
                        size="small"
                        onClick={(e) => {
                            e.stopPropagation();
                            onToggleExpanded();
                        }}
                    >
                        {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                    </IconButton>
                </Box>
            </SummarySection>

            {/* Expanded Content - Graph View */}
            <Collapse in={expanded} timeout="auto" unmountOnExit>
                <CardContent sx={{ pt: 0 }}>
                    <Stack spacing={3}>
                        {/* Graph Visualization */}
                        <GraphContainer>
                            <Typography variant="h6" sx={{ mb: 3, textAlign: 'center', color: 'text.primary' }}>
                                Request Routing Visualization
                            </Typography>

                            <GraphRow>
                                {/* Request Model Node */}
                                <NodeContainer>
                                    <Typography variant="caption" sx={{ color: 'text.secondary', mb: 1 }}>
                                        Request Local Model
                                    </Typography>
                                    <ModelNode
                                        active={record.active}
                                        label="Unspecified"
                                        value={record.requestModel}
                                        editable={record.active}
                                        onUpdate={(value) => onUpdateRecord('requestModel', value)}
                                    />
                                </NodeContainer>

                                {/* Arrow */}
                                {record.providers.length > 0 && (
                                    <ConnectionLine>
                                        <ArrowForwardIcon />
                                    </ConnectionLine>
                                )}

                                {/* Providers Container */}
                                {record.providers.length > 0 ? (
                                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
                                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                            Forwarding to Providers
                                        </Typography>
                                        <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap', justifyContent: 'center', alignItems: 'center' }}>
                                            {record.providers.map((provider) => (
                                                <ProviderNodeComponent
                                                    key={provider.uuid}
                                                    provider={provider}
                                                    apiStyle={getApiStyle(provider.provider)}
                                                    availableProviders={providers}
                                                    providerModels={providerModels}
                                                    active={record.active && provider.active !== false}
                                                    onUpdate={(field, value) => onUpdateProvider(recordUuid, provider.uuid, field, value)}
                                                    onDelete={() => onDeleteProvider(recordUuid, provider.uuid)}
                                                    onRefreshModels={() => onRefreshModels(provider.provider)}
                                                    providerUuidToName={providerUuidToName}
                                                />
                                            ))}
                                        </Box>
                                    </Box>
                                ) : (
                                    <Box sx={{ textAlign: 'center', py: 2 }}>
                                        <Typography variant="body2" color="error" gutterBottom>
                                            No providers configured
                                        </Typography>
                                        <Tooltip title="Add your first provider">
                                            <IconButton
                                                onClick={(e) => {
                                                    e.stopPropagation();
                                                    onAddProvider();
                                                }}
                                                disabled={!record.active || saving}
                                                sx={{
                                                    width: 48,
                                                    height: 48,
                                                    border: '2px dashed',
                                                    borderColor: 'divider',
                                                    borderRadius: 2,
                                                    backgroundColor: 'background.paper',
                                                    '&:hover': {
                                                        borderColor: 'primary.main',
                                                        backgroundColor: 'action.hover',
                                                        borderStyle: 'solid',
                                                    },
                                                    '&:disabled': {
                                                        borderColor: 'action.disabled',
                                                        backgroundColor: 'action.disabledBackground',
                                                    }
                                                }}
                                            >
                                                <AddIcon sx={{ fontSize: 28, color: 'text.secondary' }} />
                                            </IconButton>
                                        </Tooltip>
                                    </Box>
                                )}

                                {/* Arrow to Response */}
                                {record.providers.length > 0 && record.responseModel && (
                                    <ConnectionLine>
                                        <ArrowForwardIcon />
                                    </ConnectionLine>
                                )}

                                {/* Response Model Node - Only show if configured */}
                                {record.responseModel && (
                                    <NodeContainer>
                                        <Typography variant="caption" sx={{ color: 'text.secondary', mb: 1 }}>
                                            Response Model
                                        </Typography>
                                        <ModelNode
                                            active={record.active}
                                            label=""
                                            value={record.responseModel}
                                            editable={true} // Allow editing for consistency
                                            onUpdate={(value) => onUpdateRecord('responseModel', value)}
                                        />
                                    </NodeContainer>
                                )}
                            </GraphRow>

                            {/* Legend */}
                            <Box sx={{ display: 'flex', justifyContent: 'center', gap: 3, mt: 3, pt: 2, borderTop: '1px solid', borderColor: 'divider', flexWrap: 'wrap' }}>
                                <Typography variant="caption" color="text.secondary">
                                    • Click any node to edit
                                </Typography>
                                <Typography variant="caption" color="text.secondary" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                                    <CheckCircleIcon sx={{ fontSize: 14, color: 'success.main' }} />
                                    Active
                                </Typography>
                                <Typography variant="caption" color="text.secondary" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                                    <RadioButtonUncheckedIcon sx={{ fontSize: 14, color: 'text.disabled' }} />
                                    Inactive
                                </Typography>
                                <Typography variant="caption" color="text.secondary">
                                    • Weight affects load balancing
                                </Typography>
                            </Box>
                        </GraphContainer>

                        {/* Configuration Section - Reuse RuleCard content */}
                        <Stack spacing={2}>
                            {/* Model Configuration */}
                            <Box>
                                <Typography variant="subtitle1" sx={{ fontWeight: 600, color: 'text.primary', mb: 1.5 }}>
                                    Model Configuration
                                </Typography>
                                <Stack direction="row" spacing={1}>
                                    <TextField
                                        id={`request-model-${record.uuid}`}
                                        label="Request Model"
                                        value={record.requestModel}
                                        onChange={(e) => onUpdateRecord('requestModel', e.target.value)}
                                        helperText="Enter specified model name"
                                        size="small"
                                        disabled={!record.active}
                                        fullWidth
                                    />
                                    <TextField
                                        id={`response-model-${record.uuid}`}
                                        label="Response Model"
                                        value={record.responseModel}
                                        onChange={(e) => {
                                            onUpdateRecord('responseModel', e.target.value);
                                            if (e.target.value) {
                                                setShowResponseField(false);
                                            }
                                        }}
                                        helperText={record.responseModel ? "Model to return as" : "Leave empty for as-is response"}
                                        size="small"
                                        disabled={!record.active}
                                        sx={{
                                            minWidth: 200,
                                            display: record.responseModel || showResponseField ? 'block' : 'none'
                                        }}
                                    />
                                </Stack>
                            </Box>

                            {/* Provider Configuration */}
                            <ProviderConfig
                                providers={record.providers}
                                availableProviders={providers}
                                providerModels={providerModels}
                                providerUuidToName={providerUuidToName}
                                active={record.active}
                                onAddProvider={onAddProvider}
                                onDeleteProvider={onDeleteProvider}
                                onUpdateProvider={onUpdateProvider}
                                onRefreshModels={onRefreshModels}
                            />
                        </Stack>
                    </Stack>
                </CardContent>
            </Collapse>
        </StyledCard>
    );
};

export default RuleGraph;