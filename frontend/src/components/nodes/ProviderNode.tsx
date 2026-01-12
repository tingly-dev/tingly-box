import {
    Delete as DeleteIcon,
    MoreVert as MoreIcon,
    Refresh as RefreshIcon
} from '@mui/icons-material';
import {
    Box,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Tooltip,
    Typography
} from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import type { Provider } from '../../types/provider.ts';
import { ApiStyleBadge } from '../ApiStyleBadge.tsx';
import type { ConfigProvider } from '../RoutingGraphTypes.ts';
import { ProviderNodeContainer, providerNode } from './styles.tsx';

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
export const ProviderNode: React.FC<ProviderNodeComponentProps> = ({
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
                            flex: 10,
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
                                flex: 10,
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

                    {/* More Options Button - Moved to bottom right */}
                    <Box
                        sx={{
                            flex: 1,
                            p: providerNode.fieldPadding,
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
                    <IconButton
                        size="small"
                        onClick={handleMenuClick}
                        title={t('rule.menu.refreshModels')}
                        sx={{
                            position: 'absolute',
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
                        <MoreIcon />
                    </IconButton>
                    </Box>
                </Box>



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
