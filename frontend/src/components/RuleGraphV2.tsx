import {
    Add as AddIcon,
    ArrowBack as ArrowBackIcon,
    ArrowForward as ArrowForwardIcon,
    Delete as DeleteIcon,
    ExpandMore as ExpandMoreIcon,
    Info as InfoIcon,
    MoreVert as MoreVertIcon,
    Refresh as RefreshIcon
} from '@mui/icons-material';
import {
    Box,
    Card,
    CardContent,
    Chip,
    Collapse,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Provider } from '../types/provider';
import { ApiStyleBadge } from "./ApiStyleBadge.tsx";
import type { ConfigProvider, ConfigRecord } from './RuleGraphTypes.ts';

interface RuleGraphProps {
    record: ConfigRecord;
    providers: any[];
    providerUuidToName: { [uuid: string]: string };
    saving: boolean;
    expanded: boolean;
    collapsible?: boolean;
    recordUuid: string;
    onUpdateRecord: (field: keyof ConfigRecord, value: any) => void;
    onDeleteProvider: (recordId: string, providerId: string) => void;
    onRefreshModels: (providerUuid: string) => void;
    onToggleExpanded: () => void;
    onProviderNodeClick: (providerUuid: string) => void;
    onAddProviderButtonClick: () => void;
    extraActions?: React.ReactNode;
}

const StyledCard = styled(Card, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    transition: 'all 0.2s ease-in-out',
    opacity: active ? 1 : 0.6,
    filter: active ? 'none' : 'grayscale(0.3)',
    '&:hover': {
        boxShadow: active ? theme.shadows[4] : theme.shadows[1],
    },
}));

const SummarySection = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'collapsible',
})<{ collapsible?: boolean }>(({ theme, collapsible }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: theme.spacing(2),
    cursor: collapsible ? 'pointer' : 'default',
    ...(collapsible && {
        '&:hover': {
            backgroundColor: 'action.hover',
        },
    }),
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
    compact?: boolean;
}> = ({ active, label, value, editable = false, onUpdate, showStatusIcon = true, compact = false }) => {
    const { t } = useTranslation();
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
            <StyledModelNode compact={compact}>
                {editMode && editable ? (
                    <TextField
                        value={tempValue}
                        onChange={(e) => setTempValue(e.target.value)}
                        onBlur={handleSave}
                        onKeyDown={handleKeyDown}
                        size="small"
                        fullWidth
                        autoFocus
                        label={t('rule.card.unspecifiedModel')}
                        sx={{
                            '& .MuiInputBase-input': {
                                color: 'text.primary',
                                fontWeight: 'inherit',
                                fontSize: 'inherit',
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
                            py: compact ? 0.5 : 1.5,
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
                        {/*{showStatusIcon && (*/}
                        {/*    active ? (*/}
                        {/*        <CheckCircleIcon sx={{ fontSize: 16, color: 'success.main' }} />*/}
                        {/*    ) : (*/}
                        {/*        <RadioButtonUncheckedIcon sx={{ fontSize: 16, color: 'text.disabled' }} />*/}
                        {/*    )*/}
                        {/*)}*/}
                        {/*<EditIcon sx={{ fontWeight: 600, fontSize: '0.9rem' }}></EditIcon>*/}
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
const StyledModelNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'compact',
})<{ compact?: boolean }>(({ compact, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: theme.spacing(compact ? 1.5 : 2.5),
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    textAlign: 'center',
    width: 180,  // Fixed width
    height: compact ? 100 : 200,  // Dynamic height - half when compact
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
    providersData: Provider[];
    active: boolean;
    onDelete: () => void;
    onRefreshModels: (provider: Provider) => void;
    providerUuidToName: { [uuid: string]: string };
    onNodeClick: () => void;
}> = ({
    provider,
    apiStyle,
    providersData,
    active,
    onDelete,
    onRefreshModels,
    providerUuidToName,
    onNodeClick
}) => {
        const { t } = useTranslation();
        const [anchorEl, setAnchorEl] = React.useState<null | HTMLElement>(null);
        const menuOpen = Boolean(anchorEl);

        const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
            event.stopPropagation();
            setAnchorEl(event.currentTarget);
        };

        const handleMenuClose = () => {
            setAnchorEl(null);
        };

        const handleRefresh = (p: Provider) => {
            handleMenuClose();
            onRefreshModels(p);
        };

        const handleDelete = () => {
            handleMenuClose();
            onDelete();
        };

        // Get current provider object for display
        const currentProvider = providersData.find(p => p.uuid === provider.provider);

        return (
            <>
                <ProviderNode onClick={onNodeClick} sx={{ cursor: active ? 'pointer' : 'default' }}>
                    {/* API Style Title */}
                    {provider.provider && (
                        <Box sx={{ width: '100%', mb: 2 }}>
                            <ApiStyleBadge
                                apiStyle={apiStyle}
                                sx={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    p: 1,
                                    borderRadius: 1,
                                    transition: 'all 0.2s',
                                    width: '100%',
                                    minHeight: '32px'
                                }}
                            />
                        </Box>
                    )}

                    {/* Provider Section */}
                    <Box sx={{ width: '100%', mb: 2 }}>
                        <Box
                            sx={{
                                p: 1,
                                border: '1px solid',
                                borderColor: 'text.primary',
                                borderRadius: 1,
                                backgroundColor: 'background.paper',
                                transition: 'all 0.2s',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                gap: 0.5,
                                width: '100%',
                                minHeight: '32px'
                            }}
                        >
                            <Typography variant="body2" color="text.primary">
                                {providerUuidToName[provider.provider] || t('rule.graph.selectProvider')}
                            </Typography>
                        </Box>
                    </Box>

                    {/* Model Section */}
                    {provider.provider && (
                        <Box sx={{ width: '100%', mb: 1.5 }}>
                            <Box
                                sx={{
                                    p: 1,
                                    border: '1px dashed',
                                    borderColor: 'text.primary',
                                    borderRadius: 1,
                                    backgroundColor: 'background.paper',
                                    transition: 'all 0.2s',
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    width: '100%',
                                    minHeight: '32px'
                                }}
                            >
                                <Typography
                                    variant="body2"
                                    color="text.primary"
                                    sx={{ fontStyle: !provider.model ? 'italic' : 'normal' }}
                                >
                                    {provider.model || t('rule.graph.selectModel')}
                                </Typography>
                            </Box>
                        </Box>
                    )}

                    {/* More Options Button - Moved to bottom right */}
                    <IconButton
                        size="small"
                        onClick={handleMenuClick}
                        title={t('rule.menu.refreshModels')}
                        sx={{
                            position: 'absolute',
                            bottom: 4,
                            right: 4,
                            zIndex: 10,
                            p: 0.5,
                            opacity: 0.6,
                            color: 'text.primary',
                            '&:hover': {
                                opacity: 1,
                                backgroundColor: 'primary.main'
                            }
                        }}
                    >
                        <MoreVertIcon />
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
                        {currentProvider && (
                            <MenuItem onClick={() => {
                                handleMenuClose();
                                handleRefresh(currentProvider);
                            }} disabled={!provider.provider || !active}>
                                <ListItemIcon>
                                    <RefreshIcon />
                                </ListItemIcon>
                                <ListItemText>{t('rule.menu.refreshModels')}</ListItemText>
                            </MenuItem>
                        )}
                        <MenuItem onClick={handleDelete} disabled={!active}>
                            <ListItemIcon>
                                <DeleteIcon color="error" />
                            </ListItemIcon>
                            <ListItemText sx={{ color: 'error.main' }}>{t('rule.menu.deleteProvider')}</ListItemText>
                        </MenuItem>
                    </Menu>
                </ProviderNode>
            </>
        );
    };

// Main RuleGraph Component
const RuleGraph: React.FC<RuleGraphProps> = ({
    record,
    providers,
    providerUuidToName,
    saving,
    expanded,
    collapsible = false,
    recordUuid,
    onUpdateRecord,
    onDeleteProvider,
    onRefreshModels,
    onToggleExpanded,
    onProviderNodeClick,
    onAddProviderButtonClick,
    extraActions
}) => {
    const getApiStyle = (providerUuid: string) => {
        const provider = providers.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    };

    return (
        <StyledCard active={record.active}>
            {/* Header Section - RuleCard Style */}
            <SummarySection
                collapsible={collapsible}
                onClick={collapsible ? onToggleExpanded : undefined}
            >
                {/* Left side */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, flexGrow: 1 }}>
                    <Typography variant="h6" sx={{
                        fontWeight: 600,
                        color: record.active ? 'text.primary' : 'text.disabled'
                    }}>
                        {record.requestModel || 'Specified model name'}
                    </Typography>
                    <Chip
                        label={`Use ${record.providers.length} ${record.providers.length === 1 ? 'Key' : 'Keys'}`}
                        size="small"
                        variant="outlined"
                        onClick={(e) => e.stopPropagation()}
                        sx={{
                            opacity: record.active ? 1 : 0.5,
                            borderColor: record.active ? 'inherit' : 'text.disabled',
                            color: record.active ? 'inherit' : 'text.disabled'
                        }}
                    />
                    <Chip
                        label={record.active ? "Active" : "Inactive"}
                        size="small"
                        color={record.active ? "success" : "default"}
                        variant={record.active ? "filled" : "outlined"}
                        onClick={(e) => e.stopPropagation()}
                        sx={{
                            opacity: record.active ? 1 : 0.7,
                        }}
                    />
                    <Switch
                        checked={record.active}
                        onChange={(e) => onUpdateRecord('active', e.target.checked)}
                        disabled={saving}
                        size="small"
                        color="success"
                        onClick={(e) => e.stopPropagation()}
                    />
                </Box>
                {/* Right side */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Box onClick={(e) => e.stopPropagation()}>{extraActions}</Box>
                    {record.responseModel && <Chip
                        label={`Response as ${record.responseModel}`}
                        size="small"
                        color="info"
                        onClick={(e) => e.stopPropagation()}
                        sx={{
                            opacity: record.active ? 1 : 0.5,
                            backgroundColor: record.active ? 'info.main' : 'action.disabled',
                            color: record.active ? 'info.contrastText' : 'text.disabled'
                        }}
                    />}
                    {collapsible && (
                        <IconButton
                            size="small"
                            onClick={(e) => {
                                e.stopPropagation();
                                onToggleExpanded();
                            }}
                            sx={{
                                transition: 'transform 0.2s',
                                transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
                            }}
                        >
                            <ExpandMoreIcon />
                        </IconButton>
                    )}
                </Box>
            </SummarySection>

            {/* Expanded Content - Graph View */}
            <Collapse in={expanded} timeout="auto" unmountOnExit>
                <CardContent sx={{ pt: 0 }}>
                    <Stack spacing={3}>
                        {/* Graph Visualization */}
                        <GraphContainer>
                            <Typography variant="h6" sx={{ mb: 3, textAlign: 'center', color: 'text.primary' }}>
                                Request Proxy Visualization
                            </Typography>

                            <GraphRow>
                                {/* Model Node(s) Container */}
                                <NodeContainer>
                                    {record.responseModel ? (
                                        // Split display when response model is configured
                                        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                                            {/* Request Model Card */}
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                                <Box sx={{ flex: 1 }}>
                                                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5, mb: 1 }}>
                                                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                                            Request Local Model
                                                        </Typography>
                                                        <Tooltip title="The model name that clients use to make requests. This will be matched against incoming API calls.">
                                                            <InfoIcon sx={{ fontSize: '0.9rem', color: 'text.secondary', cursor: 'help' }} />
                                                        </Tooltip>
                                                    </Box>
                                                    <ModelNode
                                                        active={record.active}
                                                        label="Unspecified"
                                                        value={record.requestModel}
                                                        editable={record.active}
                                                        onUpdate={(value) => onUpdateRecord('requestModel', value)}
                                                        compact={true}
                                                    />
                                                </Box>
                                            </Box>

                                            {/* Response Model Card */}
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                                <Box sx={{ flex: 1 }}>
                                                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5, mb: 1 }}>
                                                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                                                            Response Model
                                                        </Typography>
                                                        <Tooltip title="The model name returned to clients. Responses from upstream providers will be transformed to show this model name instead.">
                                                            <InfoIcon sx={{ fontSize: '0.9rem', color: 'text.secondary', cursor: 'help' }} />
                                                        </Tooltip>
                                                    </Box>
                                                    <ModelNode
                                                        active={record.active}
                                                        label=""
                                                        value={record.responseModel}
                                                        editable={true}
                                                        onUpdate={(value) => onUpdateRecord('responseModel', value)}
                                                        compact={true}
                                                    />
                                                </Box>
                                            </Box>
                                        </Box>
                                    ) : (
                                        // Single display when no response model
                                        <Box>
                                            <Typography variant="caption" sx={{ color: 'text.secondary', mb: 1, textAlign: 'center', display: 'block' }}>
                                                Request Local Model
                                            </Typography>
                                            <ModelNode
                                                active={record.active}
                                                label="Unspecified"
                                                value={record.requestModel}
                                                editable={record.active}
                                                onUpdate={(value) => onUpdateRecord('requestModel', value)}
                                            />
                                        </Box>
                                    )}
                                </NodeContainer>

                                {/* Arrow from model(s) to providers */}
                                {record.providers.length > 0 && (
                                    record.responseModel ? (
                                        // When response model exists: show two rotated arrows to indicate connection
                                        <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
                                            <ConnectionLine>
                                                <ArrowForwardIcon sx={{ transform: 'rotate(45deg)' }} />
                                            </ConnectionLine>
                                            <ConnectionLine>
                                                <ArrowBackIcon sx={{ transform: 'rotate(-45deg)' }} />
                                            </ConnectionLine>
                                        </Box>
                                    ) : (
                                        // When no response model: show only forward arrow
                                        <ConnectionLine>
                                            <ArrowForwardIcon />
                                        </ConnectionLine>
                                    )
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
                                                    providersData={providers as Provider[]}
                                                    active={record.active && provider.active !== false}
                                                    onDelete={() => onDeleteProvider(recordUuid, provider.uuid)}
                                                    onRefreshModels={(p) => onRefreshModels(p.uuid)}
                                                    providerUuidToName={providerUuidToName}
                                                    onNodeClick={() => onProviderNodeClick(provider.uuid)}
                                                />
                                            ))}
                                            {/* Add Provider Button */}
                                            <Tooltip title={
                                                record.providers.length === 0
                                                    ? "Add a provider to enable request forwarding"
                                                    : record.providers.length === 1
                                                        ? "Add another provider (with 2+ providers, load balancing will be enabled based on strategy)"
                                                        : "Add another provider (requests will be load balanced across all providers)"
                                            }>
                                                <IconButton
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        onAddProviderButtonClick();
                                                    }}
                                                    disabled={!record.active || saving}
                                                    sx={{
                                                        width: 180,  // Same width as provider nodes
                                                        height: 200, // Same height as provider nodes
                                                        border: '2px dashed',
                                                        borderColor: 'divider',
                                                        borderRadius: 2,
                                                        backgroundColor: 'background.paper',
                                                        boxShadow: theme => theme.shadows[2],
                                                        transition: 'all 0.2s ease-in-out',
                                                        display: 'flex',
                                                        flexDirection: 'column',
                                                        justifyContent: 'center',
                                                        alignItems: 'center',
                                                        gap: 1,
                                                        '&:hover': {
                                                            borderColor: 'primary.main',
                                                            backgroundColor: 'action.hover',
                                                            borderStyle: 'solid',
                                                            boxShadow: theme => theme.shadows[4],
                                                            transform: 'translateY(-2px)',
                                                        },
                                                        '&:disabled': {
                                                            borderColor: 'action.disabled',
                                                            backgroundColor: 'action.disabledBackground',
                                                        }
                                                    }}
                                                >
                                                    <AddIcon sx={{ fontSize: 40, color: 'text.secondary' }} />
                                                    <Typography variant="body2" color="text.secondary" textAlign="center">
                                                        Add Provider
                                                    </Typography>
                                                </IconButton>
                                            </Tooltip>
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
                                                    onAddProviderButtonClick();
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

                            </GraphRow>

                            {/* Legend */}
                            <Box sx={{ display: 'flex', justifyContent: 'center', gap: 3, mt: 3, pt: 2, borderTop: '1px solid', borderColor: 'divider', flexWrap: 'wrap' }}>
                                <Typography variant="caption" color="text.secondary">
                                    • Click provider node to select provider and model
                                </Typography>
                            </Box>
                        </GraphContainer>
                    </Stack>
                </CardContent>
            </Collapse>
        </StyledCard>
    );
};

export default RuleGraph;