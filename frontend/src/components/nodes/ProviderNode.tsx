import {
    Delete as DeleteIcon,
    Warning as WarningIcon,
    MoreVert as MoreVertIcon,
    PlayArrow as PlayIcon
} from '@mui/icons-material';
import {
    Box,
    Button,
    Divider,
    IconButton,
    Popover,
    Stack,
    TextField,
    Tooltip,
    Typography,
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
    /** Called when the user edits this service's priority. Omit to hide the badge. */
    onPriorityChange?: (priority: number) => void;
}

// Clickable priority badge anchored to the top-left corner of the node.
// The button itself sits *inside* the node so the click target is always
// on the node (avoids the badge floating into the gap between siblings
// where adjacent nodes may swallow the click). A small CSS translate
// shifts the visual position outward so the badge still reads as
// "attached to the corner", matching the existing badge convention.
const PriorityBadgeButton = styled(Button)(({ theme }) => ({
    position: 'absolute',
    top: 4,
    left: 4,
    transform: 'translate(-30%, -30%)',
    minWidth: 0,
    width: 26,
    height: 26,
    padding: 0,
    borderRadius: '50%',
    fontSize: '0.8rem',
    fontWeight: 700,
    lineHeight: 1,
    boxShadow: theme.shadows[2],
    zIndex: 3,
    // Belt-and-suspenders: even though we stopPropagation in the handler,
    // some MUI internals re-fire click events from the wrapping span. A
    // dedicated pointer-events region keeps clicks on the badge alone.
    pointerEvents: 'auto',
}));

interface PriorityBadgeProps {
    priority: number;
    onChange: (priority: number) => void;
    active: boolean;
}

const PriorityBadge: React.FC<PriorityBadgeProps> = ({ priority, onChange, active }) => {
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [draft, setDraft] = useState<string>(String(priority || ''));

    const open = (e: React.MouseEvent<HTMLElement>) => {
        e.stopPropagation();
        setDraft(String(priority || ''));
        setAnchor(e.currentTarget);
    };
    const close = () => setAnchor(null);

    const commit = () => {
        const parsed = parseInt(draft, 10);
        const next = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
        if (next !== priority) {
            onChange(next);
        }
        close();
    };

    const label = priority > 0 ? String(priority) : '–';
    const tooltip = priority > 0
        ? `Priority ${priority} (higher = tried first). Click to change.`
        : 'No priority set. Click to assign a priority (higher = tried first).';

    return (
        <>
            <Tooltip title={tooltip} arrow placement="top">
                <span>
                    <PriorityBadgeButton
                        variant={priority > 0 ? 'contained' : 'outlined'}
                        color={priority > 0 ? 'primary' : 'inherit'}
                        size="small"
                        onClick={open}
                        disabled={!active}
                    >
                        {label}
                    </PriorityBadgeButton>
                </span>
            </Tooltip>
            <Popover
                open={Boolean(anchor)}
                anchorEl={anchor}
                onClose={close}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                onClick={(e) => e.stopPropagation()}
            >
                <Box sx={{ p: 1.5, width: 220 }}>
                    <Typography variant="caption" color="text.secondary">
                        Priority
                    </Typography>
                    <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 0.5 }}>
                        <TextField
                            type="number"
                            size="small"
                            value={draft}
                            onChange={(e) => setDraft(e.target.value)}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter') commit();
                                if (e.key === 'Escape') close();
                            }}
                            inputProps={{ min: 0, step: 1 }}
                            autoFocus
                            fullWidth
                            placeholder="0 = unset"
                        />
                        <Button size="small" variant="contained" onClick={commit}>
                            Set
                        </Button>
                    </Stack>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.75 }}>
                        Higher number runs first. Same number = parallel tier.
                    </Typography>
                </Box>
            </Popover>
        </>
    );
};

// Provider Node Component for Graph View
export const ProviderNode: React.FC<ProviderNodeComponentProps> = ({
    provider,
    apiStyle,
    providersData,
    active,
    onDelete,
    onNodeClick,
    onPriorityChange,
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
                {onPriorityChange && (
                    <PriorityBadge
                        priority={provider.priority ?? 0}
                        onChange={onPriorityChange}
                        active={active}
                    />
                )}
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

                {/* Bottom Layer - API Style Badge(s) */}
                {provider.provider && (
                    <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                        {providerInfo.provider?.api_base_openai && providerInfo.provider?.api_base_anthropic ? (
                            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5, width: '100%' }}>
                                <ApiStyleBadge
                                    apiStyle="openai"
                                    sx={{
                                        flex: 1,
                                        borderRadius: 1,
                                        transition: 'all 0.2s',
                                        fontWeight: null,
                                    }}
                                />
                                <ApiStyleBadge
                                    apiStyle="anthropic"
                                    sx={{
                                        flex: 1,
                                        borderRadius: 1,
                                        transition: 'all 0.2s',
                                        fontWeight: null,
                                    }}
                                />
                            </Box>
                        ) : (
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
                        )}
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
