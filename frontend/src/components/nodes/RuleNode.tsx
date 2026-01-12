import {
    Delete as DeleteIcon,
    MoreHoriz as MoreHorizIcon,
    Refresh as RefreshIcon
} from '@mui/icons-material';
import {
    Box,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    TextField,
    Tooltip,
    Typography
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Provider } from '../../types/provider.ts';
import { ApiStyleBadge } from "../ApiStyleBadge.tsx";
import type { ConfigProvider } from '../RoutingGraphTypes.ts';

// Model Node dimensions
const MODEL_NODE_STYLES = {
    width: 220,
    height: 90,
    heightCompact: 60,
    widthCompact: 220,
    padding: 8,
} as const;

// Provider Node dimensions
const PROVIDER_NODE_STYLES = {
    width: 320,
    height: 120,
    heightCompact: 60,
    padding: 8,
    widthCompact: 320,
    // Internal dimensions
    badgeHeight: 5,
    fieldHeight: 5,
    fieldPadding: 2,
    elementMargin: 1,
} as const;

const { modelNode, providerNode } = { modelNode: MODEL_NODE_STYLES, providerNode: PROVIDER_NODE_STYLES };

// Container for graph nodes
export const NodeContainer = styled(Box)(() => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 8,
}));

// Connection line between nodes
export const ConnectionLine = styled(Box)(({ }) => ({
    display: 'flex',
    alignItems: 'center',
    color: 'text.secondary',
    fontSize: '1.5rem',
    '& svg': {
        fontSize: '2rem',
    }
}));

// Provider node container
export const ProviderNodeContainer = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    padding: providerNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    width: providerNode.width,
    height: providerNode.height,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    '&:hover': {
        borderColor: 'primary.main',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    }
}));

// Styled model node with unified fixed size
const StyledModelNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'compact',
})<{ compact?: boolean }>(({ compact, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: modelNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    textAlign: 'center',
    width: compact?  modelNode.widthCompact: modelNode.width,
    height: compact ? modelNode.heightCompact : modelNode.height,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    cursor: 'pointer',
    '&:hover': {
        borderColor: 'primary.main',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    }
}));

// Model Node Component with editing support
export interface ModelNodeProps {
    active: boolean;
    label: string;
    value: string;
    editable?: boolean;
    onUpdate?: (value: string) => void;
    showStatusIcon?: boolean;
    compact?: boolean;
}

export const ModelNode: React.FC<ModelNodeProps> = ({
    active,
    label,
    value,
    editable = false,
    onUpdate,
    showStatusIcon = true,
    compact = false
}) => {
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

// Provider Node Component Props
export interface ProviderNodeComponentProps {
    provider: ConfigProvider;
    apiStyle: string;
    providersData: Provider[];
    active: boolean;
    onDelete: () => void;
    onRefreshModels: (provider: Provider) => void;
    providerUuidToName: { [uuid: string]: string };
    onNodeClick: () => void;
}

// Provider Node Component for Graph View
export const ProviderNodeComponent: React.FC<ProviderNodeComponentProps> = ({
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
            <ProviderNodeContainer onClick={onNodeClick} sx={{ cursor: active ? 'pointer' : 'default' }}>
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
            </ProviderNodeContainer>
        </>
    );
};
