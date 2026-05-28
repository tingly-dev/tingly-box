import {
    Delete as DeleteIcon,
    Warning as WarningIcon,
    PlayArrow as PlayIcon,
} from '@/components/icons';
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
    // Always hollow/outline style — differentiates from smart-routing's solid disks
    border: '1.5px solid',
    backgroundColor: 'transparent',
    ...(hasPriority
        ? {
              color: theme.palette.primary.main,
              borderColor: theme.palette.primary.main,
              '&:hover': active ? { borderColor: theme.palette.primary.dark, color: theme.palette.primary.dark } : {},
          }
        : {
              color: theme.palette.text.disabled,
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
    const [error, setError] = useState<string | null>(null);

    const open = (e: React.MouseEvent<HTMLElement>) => {
        e.stopPropagation();
        setDraft(String(priority || ''));
        setError(null);
        setAnchor(e.currentTarget);
    };
    const close = () => {
        setAnchor(null);
        setError(null);
    };
    const commit = () => {
        try {
            const parsed = parseInt(draft, 10);
            const next = Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
            if (next !== priority) {
                onChange(next);
            }
            close();
        } catch (err) {
            setError('Invalid priority value. Please enter a number.');
        }
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
                    aria-label={priority > 0 ? `Priority ${priority}` : 'No priority set'}
                    role="button"
                    tabIndex={active ? 0 : undefined}
                >
                    {priority > 0 ? String(priority) : null}
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
                            onChange={(e) => {
                                setDraft(e.target.value);
                                setError(null);
                            }}
                            onKeyDown={(e) => { if (e.key === 'Enter') commit(); if (e.key === 'Escape') close(); }}
                            inputProps={{ min: 0, step: 1 }}
                            autoFocus
                            fullWidth
                            placeholder="0 = unset"
                            error={!!error}
                            helperText={error}
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
                sx={{ cursor: active ? 'pointer' : 'default' }}
            >
                {!provider.provider ? (
                    <Box sx={{ ...NODE_LAYER_STYLES.topLayer }}>
                        <Typography variant="body2" color="text.secondary"
                            sx={{ ...NODE_LAYER_STYLES.typography, fontStyle: 'italic' }}>
                            Select Provider
                        </Typography>
                    </Box>
                ) : (
                    <>
                        {/* Row 1: model name — px leaves room for overlaid priority/tags */}
                        <NodeTooltip title={<Box sx={{ whiteSpace: 'pre-line' }}>{identityTooltip}</Box>} placement="top">
                            <Box sx={{ ...NODE_LAYER_STYLES.topLayer, px: '28px' }}>
                                <Typography variant="body2" noWrap sx={{
                                    ...NODE_LAYER_STYLES.typography,
                                    maxWidth: '100%', textAlign: 'center',
                                    fontStyle: !provider.model ? 'italic' : 'normal',
                                    color: provider.model ? 'text.primary' : 'text.disabled',
                                }}>
                                    {provider.model || 'select model'}
                                </Typography>
                            </Box>
                        </NodeTooltip>

                        {/* Divider — priority floats left, tags float right, both centered on the line */}
                        <Box sx={{ position: 'relative', width: '100%', display: 'flex', justifyContent: 'center', flexShrink: 0 }}>
                            <Divider sx={NODE_LAYER_STYLES.divider} />
                            {hasPriority && (
                                <Box sx={{ position: 'absolute', left: '0px', top: '50%', transform: 'translateY(-50%)', lineHeight: 0, backgroundColor: 'background.paper', px: '2px' }}>
                                    <PriorityBadge
                                        priority={provider.priority ?? 0}
                                        onChange={onPriorityChange!}
                                        active={active}
                                    />
                                </Box>
                            )}
                            <Box sx={{ position: 'absolute', right: '0px', top: '50%', transform: 'translateY(-50%)', display: 'flex', gap: '2px', backgroundColor: 'background.paper', px: '2px', lineHeight: 0 }}>
                                {hasDualApiStyle ? (
                                    <>
                                        <ApiStyleBadge apiStyle="openai" minimal sx={{ fontSize: '0.72rem', width: 22, height: 22 }} />
                                        <ApiStyleBadge apiStyle="anthropic" minimal sx={{ fontSize: '0.72rem', width: 22, height: 22 }} />
                                    </>
                                ) : (
                                    <ApiStyleBadge apiStyle={apiStyle} minimal sx={{ fontSize: '0.72rem', width: 22, height: 22 }} />
                                )}
                            </Box>
                        </Box>

                        {/* Row 2: provider name — same px inset */}
                        <Box sx={{ ...NODE_LAYER_STYLES.bottomLayer, px: '28px' }}>
                            {isProviderMissing && (
                                <WarningIcon sx={{ fontSize: '1rem', color: 'warning.main', flexShrink: 0, mr: 0.5 }} />
                            )}
                            <Typography variant="body2" noWrap
                                color={isProviderMissing ? 'warning.main' : 'text.secondary'}
                                sx={{ ...NODE_LAYER_STYLES.typography, fontWeight: 400, maxWidth: '100%', textAlign: 'center' }}>
                                {providerInfo.name}
                            </Typography>
                        </Box>
                    </>
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
