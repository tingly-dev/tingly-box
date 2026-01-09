import {
    Add as AddIcon,
    ArrowBack as ArrowBackIcon,
    ArrowForward as ArrowForwardIcon,
    Delete as DeleteIcon,
    ExpandMore as ExpandMoreIcon,
    Info as InfoIcon,
    MoreHoriz as MoreHorizIcon,
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

// Unified RuleGraph style configuration
const RULE_GRAPH_STYLES = {
    // Node dimensions
    node: {
        width: 320,
        height: 120,
        heightCompact: 60,
        padding: 10,  // Container padding
    },
    // Spacing
    spacing: {
        xs: 4,   // 0.5
        sm: 8,   // 1
        md: 12,  // 1.5
        lg: 16,  // 2
        xl: 16,  // 3
    },
    // Header
    header: {
        paddingX: 16,  // spacing(2)
        paddingY: 8,   // spacing(1)
    },
    // Graph container
    graphContainer: {
        paddingX: 16,  // spacing(2)
        paddingY: 10,  // Reduced from 12
        marginX: 16,   // spacing(2)
        marginY: 8,    // spacing(1)
    },
    // Graph content spacing
    graph: {
        stackSpacing: 0,      // Stack spacing between sections
        modelGap: 8,          // Gap between model nodes
        labelMargin: 4,       // Margin below labels
        rowGap: 16,           // Gap between graph rows
        iconGap: 4,           // Gap between icon and text
        wrapperGap: 8,        // Gap in wrapper boxes
    },
    // Model node specific
    modelNode: {
        padding: 10,          // Internal padding - same as node.padding
    },
    // Provider node internal
    providerNode: {
        badgeHeight: 5,       // API Style badge
        fieldHeight: 5,       // Provider/Model fields
        fieldPadding: 2,      // Internal padding
        elementMargin: 1,     // Margin between elements
    },
} as const;

// Shorthand for common values
const { node, spacing, header, graphContainer, graph, modelNode, providerNode } = RULE_GRAPH_STYLES;

interface RuleGraphProps {
    record: ConfigRecord;
    providers: any[];
    providerUuidToName: { [uuid: string]: string };
    saving: boolean;
    expanded: boolean;
    collapsible?: boolean;
    allowToggleRule?: boolean;
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
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: `${header.paddingY}px ${header.paddingX}px`,
    cursor: collapsible ? 'pointer' : 'default',
    ...(collapsible && {
        '&:hover': {
            backgroundColor: 'action.hover',
        },
    }),
}));

// Graph Container for expanded view
const GraphContainer = styled(Box)(({ theme }) => ({
    padding: `${graphContainer.paddingY}px ${graphContainer.paddingX}px`,
    backgroundColor: 'grey.50',
    borderRadius: theme.shape.borderRadius,
    margin: `${graphContainer.marginY}px ${graphContainer.marginX}px 0`,
}));

const GraphRow = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'stretch',  // Changed from 'center' to 'stretch' for better alignment
    justifyContent: 'center',
    gap: graph.rowGap,
    marginBottom: theme.spacing(1),
}));

const NodeContainer = styled(Box)(() => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 8,
}));

const ProviderNode = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    padding: node.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    width: node.width,
    height: node.height,
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
                        height: '100%',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        '&:hover': editable ? {
                            backgroundColor: 'action.hover',
                            borderRadius: 1,
                        } : {},
                    }}
                >
                    <Typography variant="body2" sx={{ fontWeight: 600, color: 'text.primary', fontSize: '0.9rem' }}>
                        {value || label}
                    </Typography>
                </Box>
            )}
        </StyledModelNode>
    );
};

// Styled model node with unified fixed size
const StyledModelNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'compact',
})<{ compact?: boolean }>(({ compact }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: modelNode.padding,
    borderRadius: 8,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    textAlign: 'center',
    width: node.width,
    height: compact ? node.heightCompact : node.height,
    boxShadow: '0 1px 3px rgba(0,0,0,0.12), 0 1px 2px rgba(0,0,0,0.24)',
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    cursor: 'pointer',
    '&:hover': {
        borderColor: 'primary.main',
        boxShadow: '0 4px 6px rgba(0,0,0,0.15), 0 2px 3px rgba(0,0,0,0.10)',
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
                        <Box sx={{ width: '100%', mb: providerNode.elementMargin }}>
                            <ApiStyleBadge
                                apiStyle={apiStyle}
                                sx={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    p: providerNode.fieldPadding,
                                    borderRadius: 1,
                                    transition: 'all 0.2s',
                                    width: '100%',
                                    maxHeight: providerNode.badgeHeight
                                }}
                            />
                        </Box>
                    )}

                    {/* Provider and Model in same row */}
                    <Box sx={{ width: '100%', display: 'flex', alignItems: 'center', gap: 1, mb: providerNode.elementMargin }}>
                        {/* Provider */}
                        <Box
                            sx={{
                                flex: 1,
                                p: providerNode.fieldPadding,
                                border: '1px solid',
                                borderColor: 'text.primary',
                                borderRadius: 1,
                                backgroundColor: 'background.paper',
                                transition: 'all 0.2s',
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                maxHeight: providerNode.fieldHeight,
                                overflow: 'hidden',
                            }}
                        >
                            <Tooltip title={providerUuidToName[provider.provider] || t('rule.graph.selectProvider')} arrow>
                                <Typography variant="body2" color="text.primary" noWrap sx={{ fontSize: '0.8rem', width: '100%', textAlign: 'center' }}>
                                    {providerUuidToName[provider.provider] || t('rule.graph.selectProvider')}
                                </Typography>
                            </Tooltip>
                        </Box>

                        {/* Model */}
                        {provider.provider && (
                            <Box
                                sx={{
                                    flex: 1,
                                    p: providerNode.fieldPadding,
                                    border: '1px dashed',
                                    borderColor: 'text.primary',
                                    borderRadius: 1,
                                    backgroundColor: 'background.paper',
                                    transition: 'all 0.2s',
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    maxHeight: providerNode.fieldHeight,
                                    overflow: 'hidden',
                                }}
                            >
                                <Tooltip title={provider.model || t('rule.graph.selectModel')} arrow>
                                    <Typography
                                        variant="body2"
                                        color="text.primary"
                                        noWrap
                                        sx={{ fontSize: '0.8rem', fontStyle: !provider.model ? 'italic' : 'normal', width: '100%', textAlign: 'center' }}
                                    >
                                        {provider.model || t('rule.graph.selectModel')}
                                    </Typography>
                                </Tooltip>
                            </Box>
                        )}
                    </Box>

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
                        <MoreHorizIcon />
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
    allowToggleRule = true,
    recordUuid,
    onUpdateRecord,
    onDeleteProvider,
    onRefreshModels,
    onToggleExpanded,
    onProviderNodeClick,
    onAddProviderButtonClick,
    extraActions
}) => {
    // When collapsible, parent controls expanded state (defaults to false when collapsible=true)
    // When not collapsible, always show expanded
    const isExpanded = !collapsible || expanded;
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
                        color: record.active ? 'text.primary' : 'text.disabled',
                        minWidth: 150,
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
                            color: record.active ? 'inherit' : 'text.disabled',
                            minWidth: 90,
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
                            minWidth: 75,
                        }}
                    />
                    <Switch
                        checked={record.active}
                        onChange={(e) => onUpdateRecord('active', e.target.checked)}
                        disabled={saving || !allowToggleRule}
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
                                transform: isExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
                            }}
                        >
                            <ExpandMoreIcon />
                        </IconButton>
                    )}
                </Box>
                <Box
                    onClick={(e) => e.stopPropagation()}
                    sx={{
                        width: '100%',
                        flexBasis: '100%',
                        mt: 0.5,
                        minHeight: '18px',
                    }}
                >
                    {record.description && (
                        <Typography
                            variant="body2"
                            sx={{
                                color: 'text.secondary',
                                fontSize: '0.8rem',
                                fontStyle: 'italic',
                            }}
                        >
                            {record.description}
                        </Typography>
                    )}
                </Box>
            </SummarySection>

            {/* Expanded Content - Graph View */}
            <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                <CardContent sx={{ pt: 0, pb: 1 }}>
                    <Stack spacing={graph.stackSpacing}>
                        {/* Graph Visualization */}
                        <GraphContainer>
                            <GraphRow>
                                {/* Model Node(s) Container */}
                                <NodeContainer>
                                    {record.responseModel ? (
                                        // Split display when response model is configured
                                        <Box sx={{ display: 'flex', flexDirection: 'column', gap: graph.modelGap }}>
                                            {/* Request Model Card */}
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: graph.wrapperGap }}>
                                                <Box sx={{ flex: 1 }}>
                                                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: graph.iconGap, mb: graph.labelMargin }}>
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
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: graph.wrapperGap }}>
                                                <Box sx={{ flex: 1 }}>
                                                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: graph.iconGap, mb: graph.labelMargin }}>
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
                                            <Typography variant="caption" sx={{ color: 'text.secondary', mb: graph.labelMargin, textAlign: 'center', display: 'block' }}>
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
                                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
                                        <Typography variant="caption" sx={{ color: 'text.secondary', mb: graph.labelMargin }}>
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
                                                        width: node.width,
                                                        height: node.height,
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
                        </GraphContainer>
                    </Stack>
                </CardContent>
            </Collapse>
        </StyledCard>
    );
};

export default RuleGraph;