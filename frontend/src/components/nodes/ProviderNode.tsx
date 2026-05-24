import {
    Delete as DeleteIcon,
    Warning as WarningIcon,
    MoreVert as MoreVertIcon,
    PlayArrow as PlayIcon,
    HorizontalRule as HorizontalRuleIcon,
} from '@mui/icons-material';
import {
    Box,
    Button,
    Divider,
    IconButton,
    Popover,
    Stack,
    TextField,
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
import NodeTooltip from './NodeTooltip.tsx';

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
//
// Implementation note: this mirrors the existing SmartOpNode index badge
// — a styled `Box` rather than a `Button`. When a `Button` is wrapped in
// `Tooltip`, MUI injects a `<span>` anchor that sizes to the rendered
// flow of its child; an absolutely-positioned button has zero flow size,
// so the span's hit region collapses and the visually-overflowing
// portion of the badge becomes unresponsive to hover/click. Using a
// `Box` with its own onClick and putting the `position: absolute` on a
// non-Tooltip wrapper keeps the visible disk and the hit region in sync.
const PriorityBadgeAnchor = styled(Box)({
    position: 'absolute',
    top: -10,
    left: -10,
    width: 26,
    height: 26,
    zIndex: 3,
    pointerEvents: 'auto',
});

const PriorityBadgeDisk = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'hasPriority' && prop !== 'active',
})<{ hasPriority: boolean; active: boolean }>(({ theme, hasPriority, active }) => ({
    width: '100%',
    height: '100%',
    borderRadius: '50%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: '0.8rem',
    fontWeight: 700,
    lineHeight: 1,
    border: '1px solid',
    boxShadow: theme.shadows[2],
    userSelect: 'none',
    cursor: active ? 'pointer' : 'not-allowed',
    transition: 'background-color 0.15s, border-color 0.15s, color 0.15s',
    ...(hasPriority
        ? {
              backgroundColor: theme.palette.primary.main,
              color: theme.palette.primary.contrastText,
              borderColor: theme.palette.primary.main,
              '&:hover': active
                  ? {
                        backgroundColor: theme.palette.primary.dark,
                        borderColor: theme.palette.primary.dark,
                    }
                  : {},
          }
        : {
              // Slightly lifted surface + 1.5px text.disabled border so the
              // disk stays legible against both the node card (light) and the
              // dark Paper surface where a thin `divider` collapses into
              // invisibility. Without this lift the no-priority badge merges
              // into the node card in dark mode.
              border: '1.5px solid',
              backgroundColor: theme.palette.background.paper,
              color: theme.palette.text.secondary,
              borderColor: theme.palette.text.disabled,
              '&:hover': active
                  ? {
                        borderColor: theme.palette.primary.main,
                        color: theme.palette.primary.main,
                    }
                  : {},
          }),
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

    const tooltip = priority > 0
        ? `Priority ${priority} (higher = tried first). Click to change.`
        : 'No priority set. Click to assign a priority (higher = tried first).';

    return (
        <>
            <PriorityBadgeAnchor>
                <NodeTooltip title={tooltip} placement="left">
                    <PriorityBadgeDisk
                        hasPriority={priority > 0}
                        active={active}
                        onClick={active ? open : undefined}
                    >
                        {priority > 0 ? String(priority) : <HorizontalRuleIcon sx={{ fontSize: 15 }} />}
                    </PriorityBadgeDisk>
                </NodeTooltip>
            </PriorityBadgeAnchor>
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

    const hasDualApiStyle = !!(
        providerInfo.provider?.api_base_openai && providerInfo.provider?.api_base_anthropic
    );
    const apiStyleLabel = hasDualApiStyle ? 'openai / anthropic' : apiStyle;

    const identityTooltip = (() => {
        if (isProviderMissing) {
            return 'Provider not found. Please refresh the page or re-import the provider.';
        }
        if (!provider.provider) return 'Select Provider';
        const modelLine = provider.model ? `Model: ${provider.model}` : 'Model: (select model)';
        const styleLine = apiStyleLabel ? `API Style: ${apiStyleLabel}` : '';
        return [`Provider: ${providerInfo.name}`, modelLine, styleLine]
            .filter(Boolean)
            .join('\n');
    })();

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
                    <NodeTooltip
                        title={<Box sx={{ whiteSpace: 'pre-line' }}>{identityTooltip}</Box>}
                        placement="top"
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5 }}>
                            {isProviderMissing && (
                                <WarningIcon sx={{ fontSize: '1rem', color: 'warning.main' }} />
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
                    </NodeTooltip>
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
                        <NodeTooltip title="Test Provider" placement="bottom">
                            <IconButton
                                size="small"
                                onClick={handleProbeClick}
                                sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                            >
                                <PlayIcon sx={{ fontSize: '1rem', color: 'success.main' }} />
                            </IconButton>
                        </NodeTooltip>
                    )}
                    {/* Delete Button */}
                    <NodeTooltip title="Delete Provider" placement="bottom">
                        <IconButton
                            size="small"
                            onClick={handleMenuClick}
                            sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                        >
                            <DeleteIcon sx={{ fontSize: '1rem', color: 'error.main' }} />
                        </IconButton>
                    </NodeTooltip>
                </ActionButtonsBox>
            </ProviderNodeContainer>
        </ProviderNodeWrapper>
    );
};
