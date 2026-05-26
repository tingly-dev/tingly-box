import {
    Delete as DeleteIcon,
    Warning as WarningIcon,
    PlayArrow as PlayIcon,
    HorizontalRule as HorizontalRuleIcon,
} from '@/components/icons';
import {
    Box,
    Button,
    IconButton,
    Popover,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { alpha, styled } from '@mui/material/styles';
import React, { useState } from 'react';
import type { Provider } from '@/types/provider.ts';
import { ApiStyleBadge } from '../ApiStyleBadge.tsx';
import { ProbeV2Menu } from '../probe';
import type { ConfigProvider } from '../RoutingGraphTypes.ts';
import { ProviderNodeContainer, NODE_LAYER_STYLES } from './styles.tsx';
import ProviderNodeContent from './ProviderNodeContent.tsx';
import NodeTooltip from './NodeTooltip.tsx';

const ActionButtonsBox = styled(Box)(() => ({
    position: 'absolute',
    top: 4,
    right: 4,
    display: 'flex',
    gap: 2,
    opacity: 0,
    transition: 'opacity 0.2s',
}));

const ProviderNodeWrapper = styled(Box)(() => ({
    position: 'relative',
    '&:hover .action-buttons': { opacity: 1 },
}));

// Inline priority disk — lives inside the left column of the node, no overflow.
const PriorityDisk = styled(Box, {
    shouldForwardProp: (p) => p !== 'hasPriority' && p !== 'active',
})<{ hasPriority: boolean; active: boolean }>(({ theme, hasPriority, active }) => ({
    width: 22,
    height: 22,
    borderRadius: '50%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: '0.72rem',
    fontWeight: 700,
    lineHeight: 1,
    userSelect: 'none',
    cursor: active ? 'pointer' : 'not-allowed',
    transition: 'background-color 0.15s, border-color 0.15s, color 0.15s',
    ...(hasPriority
        ? {
              border: '1px solid',
              backgroundColor: theme.palette.primary.main,
              color: theme.palette.primary.contrastText,
              borderColor: theme.palette.primary.main,
              '&:hover': active ? { backgroundColor: theme.palette.primary.dark, borderColor: theme.palette.primary.dark } : {},
          }
        : {
              border: '1.5px solid',
              backgroundColor: theme.palette.background.paper,
              color: theme.palette.text.secondary,
              borderColor: theme.palette.text.disabled,
              '&:hover': active ? { borderColor: theme.palette.primary.main, color: theme.palette.primary.main } : {},
          }),
}));

const getProviderInfo = (providerUuid: string, providersData: Provider[]) => {
    const provider = providersData.find(p => p.uuid === providerUuid);
    return { name: provider?.name || 'Unknown Provider', exists: !!provider, provider };
};

export interface ProviderNodeComponentProps {
    provider: ConfigProvider;
    apiStyle: string;
    providersData: Provider[];
    active: boolean;
    onDelete: () => void;
    onNodeClick: () => void;
    onPriorityChange?: (priority: number) => void;
}

interface PriorityBadgeProps {
    priority: number;
    onChange: (priority: number) => void;
    active: boolean;
}

const PriorityBadge: React.FC<PriorityBadgeProps> = ({ priority, onChange, active }) => {
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [draft, setDraft] = useState(String(priority || ''));

    const open = (e: React.MouseEvent<HTMLElement>) => {
        e.stopPropagation();
        setDraft(String(priority || ''));
        setAnchor(e.currentTarget);
    };
    const close = () => setAnchor(null);
    const commit = () => {
        const parsed = parseInt(draft, 10);
        const next = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
        if (next !== priority) onChange(next);
        close();
    };

    const tooltip = priority > 0
        ? `Priority ${priority} (higher = tried first). Click to change.`
        : 'No priority set. Click to assign a priority.';

    return (
        <>
            <NodeTooltip title={tooltip} placement="left">
                <PriorityDisk
                    hasPriority={priority > 0}
                    active={active}
                    onClick={active ? open : undefined}
                >
                    {priority > 0 ? String(priority) : <HorizontalRuleIcon sx={{ fontSize: 13 }} />}
                </PriorityDisk>
            </NodeTooltip>
            <Popover
                open={Boolean(anchor)}
                anchorEl={anchor}
                onClose={close}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                onClick={(e) => e.stopPropagation()}
            >
                <Box sx={{ p: 1.5, width: 220 }}>
                    <Typography variant="caption" color="text.secondary">Priority</Typography>
                    <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 0.5 }}>
                        <TextField
                            type="number"
                            size="small"
                            value={draft}
                            onChange={(e) => setDraft(e.target.value)}
                            onKeyDown={(e) => { if (e.key === 'Enter') commit(); if (e.key === 'Escape') close(); }}
                            inputProps={{ min: 0, step: 1 }}
                            autoFocus
                            fullWidth
                            placeholder="0 = unset"
                        />
                        <Button size="small" variant="contained" onClick={commit}>Set</Button>
                    </Stack>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.75 }}>
                        Higher number runs first. Same number = parallel tier.
                    </Typography>
                </Box>
            </Popover>
        </>
    );
};

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
    const hasDualApiStyle = !!(providerInfo.provider?.api_base_openai && providerInfo.provider?.api_base_anthropic);
    const apiStyleLabel = hasDualApiStyle ? 'openai / anthropic' : apiStyle;

    const identityTooltip = (() => {
        if (isProviderMissing) return 'Provider not found. Please refresh or re-import.';
        if (!provider.provider) return 'Select Provider';
        const modelLine = provider.model ? `Model: ${provider.model}` : 'Model: (select model)';
        const styleLine = apiStyleLabel ? `API Style: ${apiStyleLabel}` : '';
        return [`Provider: ${providerInfo.name}`, modelLine, styleLine].filter(Boolean).join('\n');
    })();

    const handleMenuClick = (e: React.MouseEvent<HTMLElement>) => { e.stopPropagation(); setMenuAnchorEl(e.currentTarget); };
    const handleMenuClose = () => setMenuAnchorEl(null);
    const handleDelete = () => { handleMenuClose(); onDelete(); };
    const handleProbeClick = (e: React.MouseEvent<HTMLElement>) => { e.stopPropagation(); setProbeAnchorEl(e.currentTarget); };
    const handleProbeClose = () => setProbeAnchorEl(null);

    const hasPriority = !!onPriorityChange;

    return (
        <ProviderNodeWrapper>
            <ProviderNodeContent
                menuAnchorEl={menuAnchorEl}
                menuOpen={menuOpen}
                onMenuClose={handleMenuClose}
                onDelete={handleDelete}
            />

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

            <ProviderNodeContainer
                onClick={onNodeClick}
                sx={{
                    cursor: active ? 'pointer' : 'default',
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'stretch',
                    p: 0,
                    overflow: 'visible',
                }}
            >
                {/* Left column: inline priority badge */}
                {hasPriority && (
                    <Box
                        sx={{
                            width: 32,
                            flexShrink: 0,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            borderRight: '1px solid',
                            borderColor: 'divider',
                        }}
                    >
                        <PriorityBadge
                            priority={provider.priority ?? 0}
                            onChange={onPriorityChange!}
                            active={active}
                        />
                    </Box>
                )}

                {/* Content: provider name (row 1) + model (row 2) */}
                <Box
                    sx={{
                        flex: 1,
                        display: 'flex',
                        flexDirection: 'column',
                        minWidth: 0,
                        pl: hasPriority ? 1 : 1.5,
                        // leave room on the right so text doesn't run into the style tag area
                        pr: provider.provider ? (hasDualApiStyle ? 1 : 0.5) : 1,
                    }}
                >
                    {!provider.provider ? (
                        <Box sx={{ flex: 1, display: 'flex', alignItems: 'center' }}>
                            <Typography
                                variant="body2"
                                color="text.secondary"
                                sx={{ ...NODE_LAYER_STYLES.typography, fontStyle: 'italic' }}
                            >
                                Select Provider
                            </Typography>
                        </Box>
                    ) : (
                        <>
                            {/* Row 1: provider name */}
                            <NodeTooltip
                                title={<Box sx={{ whiteSpace: 'pre-line' }}>{identityTooltip}</Box>}
                                placement="top"
                            >
                                <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                                    {isProviderMissing && (
                                        <WarningIcon sx={{ fontSize: '1rem', color: 'warning.main', flexShrink: 0 }} />
                                    )}
                                    <Typography
                                        variant="body2"
                                        color={isProviderMissing ? 'warning.main' : 'text.primary'}
                                        noWrap
                                        sx={{ ...NODE_LAYER_STYLES.typography, flex: 1, minWidth: 0 }}
                                    >
                                        {providerInfo.name}
                                    </Typography>
                                </Box>
                            </NodeTooltip>

                            {/* Row 2: model name */}
                            <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', minWidth: 0 }}>
                                <Typography
                                    variant="body2"
                                    noWrap
                                    sx={{
                                        ...NODE_LAYER_STYLES.typography,
                                        fontWeight: 400,
                                        fontStyle: !provider.model ? 'italic' : 'normal',
                                        color: provider.model ? 'text.secondary' : 'text.disabled',
                                        flex: 1,
                                        minWidth: 0,
                                    }}
                                >
                                    {provider.model || 'select model'}
                                </Typography>
                            </Box>
                        </>
                    )}
                </Box>

                {/* Style tag(s): bottom-right corner, bleeding out */}
                {provider.provider && (
                    <Box
                        sx={{
                            position: 'absolute',
                            bottom: -8,
                            right: -8,
                            display: 'flex',
                            gap: '2px',
                            zIndex: 2,
                        }}
                    >
                        {hasDualApiStyle ? (
                            <>
                                <ApiStyleBadge apiStyle="openai" minimal />
                                <ApiStyleBadge apiStyle="anthropic" minimal />
                            </>
                        ) : (
                            <ApiStyleBadge apiStyle={apiStyle} minimal />
                        )}
                    </Box>
                )}

                {/* Action buttons (hover) */}
                <ActionButtonsBox className="action-buttons">
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
