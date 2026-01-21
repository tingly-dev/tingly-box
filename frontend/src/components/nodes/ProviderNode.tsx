import {
    Delete as DeleteIcon
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
import type { Provider } from '../../types/provider.ts';
import { ApiStyleBadge } from '../ApiStyleBadge.tsx';
import type { ConfigProvider } from '../RoutingGraphTypes.ts';
import { ProviderNodeContainer, providerNode } from './styles.tsx';

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

// Helper function to get provider name from providersData
const getProviderName = (providerUuid: string, providersData: Provider[]): string => {
    const provider = providersData.find(p => p.uuid === providerUuid);
    return provider?.name || 'Unknown Provider';
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
        console.log('ProviderNode handleMenuClick, active:', active);
        event.stopPropagation();
        setAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        console.log('ProviderNode handleMenuClose');
        setAnchorEl(null);
    };

    const handleDelete = () => {
        console.log('ProviderNode handleDelete');
        handleMenuClose();
        onDelete();
    };

    const providerName = getProviderName(provider.provider, providersData);

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
                        <DeleteIcon color="error" />
                    </ListItemIcon>
                    <ListItemText sx={{ color: 'error.main' }}>{t('rule.menu.deleteProvider')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={handleMenuClose} sx={{ color: 'text.secondary' }}>
                    <ListItemText>Cancel</ListItemText>
                </MenuItem>
            </Menu>
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

                {/* Combined Provider/Model Field */}
                <Box sx={{ width: '100%', mb: providerNode.elementMargin }}>
                    <Box
                        sx={{
                            width: '100%',
                            p: providerNode.fieldPadding,
                            pr: 0.5,
                            border: '1px solid',
                            borderColor: 'text.primary',
                            borderRadius: 1,
                            backgroundColor: 'background.paper',
                            transition: 'all 0.2s',
                            display: 'flex',
                            alignItems: 'center',
                            maxHeight: providerNode.fieldHeight,
                            overflow: 'hidden',
                        }}
                    >
                        <Tooltip title={
                            provider.provider && provider.model
                                ? <>Credential: {providerName}<br/>Model: {provider.model}</>
                                : provider.provider
                                    ? <>Credential: {providerName}<br/>Model: (select model)</>
                                    : t('rule.graph.selectProvider')
                        } arrow>
                            <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5 }}>
                                <Typography
                                    variant="body2"
                                    color="text.primary"
                                    noWrap
                                    sx={{
                                        fontSize: '0.8rem',
                                        fontStyle: !provider.provider ? 'italic' : 'normal',
                                    }}
                                >
                                    {providerName || t('rule.graph.selectProvider')}
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
                                            fontSize: '0.8rem',
                                            fontStyle: !provider.model ? 'italic' : 'normal',
                                        }}
                                    >
                                        {provider.model || '?'}
                                    </Typography>
                                )}
                            </Box>
                        </Tooltip>
                    </Box>
                </Box>

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
