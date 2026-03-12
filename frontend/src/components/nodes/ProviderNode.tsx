import {
    Delete as DeleteIcon,
    Warning as WarningIcon
} from '@mui/icons-material';
import {
    Box,
    Divider,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Tooltip,
    Typography
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import type { Provider } from '@/types/provider.ts';
import { ApiStyleBadge } from '../ApiStyleBadge.tsx';
import type { ConfigProvider } from '../RoutingGraphTypes.ts';
import { ProviderNodeContainer, providerNode, NODE_LAYER_STYLES } from './styles.tsx';

// Action button container
const ActionButtonsBox = styled(Box)(({ theme }) => ({
    position: 'absolute',
    top: 4,
    right: 4,
    display: 'flex',
    gap: 2,
    opacity: 0,
    transition: 'opacity 0.2s',
}));

const ProviderNodeWrapper = styled(Box)(({ theme }) => ({
    position: 'relative',
    '&:hover .action-buttons': {
        opacity: 1,
    }
}));

// Helper function to get provider info from providersData
const getProviderInfo = (providerUuid: string, providersData: Provider[]) => {
    const provider = providersData.find(p => p.uuid === providerUuid);
    return {
        name: provider?.name || 'Unknown Provider',
        exists: !!provider,
        provider
    };
};

// Provider Node Component Props
export interface ProviderNodeComponentProps {
    provider: ConfigProvider;
    apiStyle: string;
    providersData: Provider[];
    active: boolean;
    onDelete: () => void;
    onNodeClick: () => void;
}

// Provider Node Component for Graph View
export const ProviderNode: React.FC<ProviderNodeComponentProps> = ({
    provider,
    apiStyle,
    providersData,
    active,
    onDelete,
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

    const handleDelete = () => {
        handleMenuClose();
        onDelete();
    };

    const providerInfo = getProviderInfo(provider.provider, providersData);
    const isProviderMissing = provider.provider && !providerInfo.exists;

    return (
        <ProviderNodeWrapper>
            <Menu
                anchorEl={anchorEl}
                open={menuOpen}
                onClose={handleMenuClose}
                onClick={(e) => e.stopPropagation()}
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
            >
                <MenuItem onClick={handleDelete}>
                    <ListItemIcon>
                        <DeleteIcon />
                    </ListItemIcon>
                    <ListItemText>{t('rule.menu.deleteProvider')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={handleMenuClose} sx={{ color: 'text.secondary' }}>
                    <ListItemText>Cancel</ListItemText>
                </MenuItem>
            </Menu>
            <ProviderNodeContainer onClick={onNodeClick} sx={{ cursor: active ? 'pointer' : 'default', display: 'flex', flexDirection: 'column' }}>
                {/* Top Layer - Provider/Model Field */}
                <Box sx={NODE_LAYER_STYLES.topLayer}>
                    <Tooltip title={
                        provider.provider && provider.model
                            ? <>Provider: {providerInfo.name}<br/>Model: {provider.model}</>
                            : provider.provider
                                ? <>Provider: {providerInfo.name}<br/>Model: (select model)</>
                                : t('rule.graph.selectProvider')
                    } arrow>
                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5 }}>
                            {isProviderMissing && (
                                <Tooltip title="Provider not found. Please refresh the page or re-import the provider." arrow>
                                    <WarningIcon sx={{ fontSize: '1rem', color: 'warning.main' }} />
                                </Tooltip>
                            )}
                            <Typography
                                variant="body2"
                                color={isProviderMissing ? 'warning.main' : 'text.primary'}
                                noWrap
                                sx={{
                                    ...NODE_LAYER_STYLES.typography,
                                    fontStyle: !provider.provider ? 'italic' : 'normal',
                                    width: '80px',
                                    textAlign: 'center',
                                }}
                            >
                                {providerInfo.name || t('rule.graph.selectProvider')}
                            </Typography>

                            {provider.provider && (
                                <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
                            )}

                            {provider.provider && (
                                <Typography
                                    variant="body2"
                                    color="text.primary"
                                    noWrap
                                    sx={{
                                        ...NODE_LAYER_STYLES.typography,
                                        fontStyle: !provider.model ? 'italic' : 'normal',
                                        width: '80px',
                                        textAlign: 'center',
                                    }}
                                >
                                    {provider.model || '?'}
                                </Typography>
                            )}
                        </Box>
                    </Tooltip>
                </Box>

                {/* Divider */}
                <Divider sx={NODE_LAYER_STYLES.divider} />

                {/* Bottom Layer - API Style Badge */}
                {provider.provider && (
                    <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                        <ApiStyleBadge
                            apiStyle={apiStyle}
                            sx={{
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                borderRadius: 1,
                                transition: 'all 0.2s',
                                width: '100%',
                                fontWeight: null,
                            }}
                        />
                    </Box>
                )}

                {/* Action Buttons - visible on hover */}
                <ActionButtonsBox className="action-buttons">
                    <Tooltip title={t('rule.menu.deleteProvider')}>
                        <IconButton
                            size="small"
                            onClick={handleMenuClick}
                            sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                        >
                            <DeleteIcon sx={{ fontSize: '1rem', color: 'error.main' }} />
                        </IconButton>
                    </Tooltip>
                </ActionButtonsBox>
            </ProviderNodeContainer>
        </ProviderNodeWrapper>
    );
};
