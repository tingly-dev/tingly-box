import {
    Delete as DeleteIcon,
    Warning as WarningIcon,
    MoreVert as MoreVertIcon,
    PlayArrow as PlayIcon
} from '@mui/icons-material';
import {
    Box,
    Divider,
    IconButton,
    Tooltip,
    Typography
} from '@mui/material';
import { styled } from '@mui/material/styles';
import React, { useState } from 'react';
import type { Provider } from '@/types/provider.ts';
import { ApiStyleBadge } from '../ApiStyleBadge.tsx';
import { ProbeV2Menu } from '../probe';
import type { ConfigProvider } from '../RoutingGraphTypes.ts';
import { ProviderNodeContainer, NODE_LAYER_STYLES } from './styles.tsx';
import ProviderNodeContent from './ProviderNodeContent.tsx';

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
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [probeAnchorEl, setProbeAnchorEl] = useState<null | HTMLElement>(null);
    const menuOpen = Boolean(menuAnchorEl);
    const probeMenuOpen = Boolean(probeAnchorEl);

    const providerInfo = getProviderInfo(provider.provider, providersData);
    const isProviderMissing = provider.provider && !providerInfo.exists;

    const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setMenuAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        setMenuAnchorEl(null);
    };

    const handleDelete = () => {
        handleMenuClose();
        onDelete();
    };

    const handleProbeClick = (event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setProbeAnchorEl(event.currentTarget);
    };

    const handleProbeClose = () => {
        setProbeAnchorEl(null);
    };

    return (
        <ProviderNodeWrapper>
            {/* Delete Menu */}
            <ProviderNodeContent
                menuAnchorEl={menuAnchorEl}
                menuOpen={menuOpen}
                onMenuClose={handleMenuClose}
                onDelete={handleDelete}
            />

            {/* Probe Menu */}
            {provider.provider && providerInfo.exists && (
                <ProbeV2Menu
                    anchorEl={probeAnchorEl}
                    open={probeMenuOpen}
                    onClose={handleProbeClose}
                    targetType="provider"
                    targetId={provider.provider}
                    targetName={providerInfo.name}
                    model={provider.model}
                />
            )}

            <ProviderNodeContainer onClick={onNodeClick} sx={{ cursor: active ? 'pointer' : 'default', display: 'flex', flexDirection: 'column' }}>
                {/* Top Layer - Provider/Model Field */}
                <Box sx={NODE_LAYER_STYLES.topLayer}>
                    <Tooltip title={
                        provider.provider && provider.model
                            ? `Provider: ${providerInfo.name}\nModel: ${provider.model}`
                            : provider.provider
                                ? `Provider: ${providerInfo.name}\nModel: (select model)`
                                : 'Select Provider'
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
                                {providerInfo.name || 'Select Provider'}
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
                    {/* Probe Button */}
                    {provider.provider && providerInfo.exists && (
                        <Tooltip title="Test Provider">
                            <IconButton
                                size="small"
                                onClick={handleProbeClick}
                                sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                            >
                                <PlayIcon sx={{ fontSize: '1rem', color: 'success.main' }} />
                            </IconButton>
                        </Tooltip>
                    )}
                    {/* Delete Button */}
                    <Tooltip title="Delete Provider">
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
