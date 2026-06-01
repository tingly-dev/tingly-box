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
import { useTranslation } from 'react-i18next';
import type { Provider } from '@/types/provider.ts';
import { ApiStyleBadge } from '../ApiStyleBadge.tsx';
import { ProbeV2Menu } from '../probe';
import type { ConfigProvider } from '../RoutingGraphTypes.ts';
import { ServiceNodeContainer, NODE_LAYER_STYLES } from './styles.tsx';
import ServiceNodeContent from './ServiceNodeContent.tsx';
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

const ServiceNodeWrapper = styled(Box)(() => ({
    position: 'relative',
    '&:hover .action-buttons': { opacity: 1 },
}));

// Inline priority disk — lives inside the left column of the node, no overflow.
const PriorityDisk = styled(Box, {
    shouldForwardProp: (p) => p !== 'active',
})<{ active: boolean }>(({ theme, active }) => ({
    width: 24,
    height: 24,
    borderRadius: '50%',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: '0.75rem',
    fontWeight: 700,
    lineHeight: 1,
    userSelect: 'none',
    cursor: active ? 'pointer' : 'not-allowed',
    transition: 'background-color 0.15s, border-color 0.15s, color 0.15s',
    border: '1.5px solid',
    backgroundColor: 'transparent',
    color: theme.palette.primary.main,
    borderColor: theme.palette.primary.main,
    '&:hover': active ? { borderColor: theme.palette.primary.dark, color: theme.palette.primary.dark } : {},
}));

const getProviderInfo = (providerUuid: string, providersData: Provider[]) => {
    const provider = providersData.find(p => p.uuid === providerUuid);
    return { name: provider?.name || 'Unknown Provider', exists: !!provider, provider };
};

export interface ServiceNodeProps {
    provider: ConfigProvider;
    apiStyle: string;
    providersData: Provider[];
    active: boolean;
    onDelete: () => void;
    onNodeClick: () => void;
    onPriorityChange?: (priority: number) => void;
}

/** @deprecated Use ServiceNodeProps */
export type ProviderNodeComponentProps = ServiceNodeProps;

interface PriorityBadgeProps {
    priority: number;
    onChange: (priority: number) => void;
    active: boolean;
}

const PriorityBadge: React.FC<PriorityBadgeProps> = ({ priority, onChange, active }) => {
    const { t } = useTranslation();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [draft, setDraft] = useState(String(priority ?? 0));
    const [error, setError] = useState<string | null>(null);

    const open = (e: React.MouseEvent<HTMLElement>) => {
        e.stopPropagation();
        setDraft(String(priority ?? 0));
        setError(null);
        setAnchor(e.currentTarget);
    };
    const close = () => {
        setAnchor(null);
        setError(null);
    };
    const commit = () => {
        const parsed = parseInt(draft, 10);
        if (!Number.isFinite(parsed) || parsed < 0) {
            setError(t('rule.priority.invalidInput'));
            return;
        }
        if (parsed !== priority) onChange(parsed);
        close();
    };

    const tooltip = priority > 0
        ? t('rule.priority.tooltipSet', { priority })
        : t('rule.priority.tooltipUnset');

    return (
        <>
            <NodeTooltip title={tooltip} placement="left">
                <PriorityDisk
                    active={active}
                    onClick={active ? open : undefined}
                    aria-label={priority > 0 ? t('rule.priority.ariaLabel', { priority }) : t('rule.priority.ariaUnset')}
                    role="button"
                    tabIndex={active ? 0 : undefined}
                >
                    {String(priority ?? 0)}
                </PriorityDisk>
            </NodeTooltip>
            <Popover
                open={Boolean(anchor)}
                anchorEl={anchor}
                onClose={close}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                onClick={(e) => e.stopPropagation()}
            >
                <Box sx={{ p: 2, width: 240 }}>
                    <Typography variant="subtitle2" sx={{ mb: 1, fontWeight: 700 }}>
                        {t('rule.priority.editTitle')}
                    </Typography>
                    <Stack direction="row" spacing={1} alignItems="flex-start">
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
                            placeholder="0"
                            error={!!error}
                            helperText={error}
                        />
                        <Button size="small" variant="contained" onClick={commit} sx={{ mt: 0, flexShrink: 0 }}>
                            {t('common.confirm')}
                        </Button>
                    </Stack>
                    <Box sx={{ mt: 1.25, p: 1.25, borderRadius: 1, bgcolor: 'action.hover' }}>
                        <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.6 }}>
                            {t('rule.priority.helpHigher')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, lineHeight: 1.6 }}>
                            {t('rule.priority.helpZero')}
                        </Typography>
                    </Box>
                </Box>
            </Popover>
        </>
    );
};

export const ServiceNode: React.FC<ServiceNodeProps> = ({
    provider,
    apiStyle,
    providersData,
    active,
    onDelete,
    onNodeClick,
    onPriorityChange,
}) => {
    const { t } = useTranslation();
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [probeAnchorEl, setProbeAnchorEl] = useState<null | HTMLElement>(null);
    const menuOpen = Boolean(menuAnchorEl);
    const probeMenuOpen = Boolean(probeAnchorEl);

    const providerInfo = getProviderInfo(provider.provider, providersData);
    const isProviderMissing = provider.provider && !providerInfo.exists;
    const hasDualApiStyle = !!(providerInfo.provider?.api_base_openai && providerInfo.provider?.api_base_anthropic);
    const apiStyleLabel = hasDualApiStyle ? 'openai / anthropic' : apiStyle;

    const identityTooltip = (() => {
        if (isProviderMissing) return t('rule.service.providerNotFound');
        if (!provider.provider) return t('rule.service.selectProvider');
        const modelLine = provider.model
            ? `Model: ${provider.model}`
            : `Model: (${t('rule.service.selectModel')})`;
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
        <ServiceNodeWrapper>
            <ServiceNodeContent
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

            <ServiceNodeContainer
                onClick={onNodeClick}
                sx={{ cursor: active ? 'pointer' : 'default' }}
            >
                {!provider.provider ? (
                    <Box sx={{ ...NODE_LAYER_STYLES.topLayer }}>
                        <Typography variant="body2" color="text.secondary"
                            sx={{ ...NODE_LAYER_STYLES.typography, fontStyle: 'italic' }}>
                            {t('rule.service.selectProvider')}
                        </Typography>
                    </Box>
                ) : (
                    <>
                        {/* Row 1: priority disk (left) + model name (center) */}
                        <NodeTooltip title={<Box sx={{ whiteSpace: 'pre-line' }}>{identityTooltip}</Box>} placement="top">
                            <Box sx={{ ...NODE_LAYER_STYLES.topLayer, position: 'relative', px: '28px' }}>
                                {hasPriority && (
                                    <Box sx={{ position: 'absolute', left: 0, top: '50%', transform: 'translateY(-50%)', lineHeight: 0 }}>
                                        <PriorityBadge
                                            priority={provider.priority ?? 0}
                                            onChange={onPriorityChange!}
                                            active={active}
                                        />
                                    </Box>
                                )}
                                <Typography variant="body2" noWrap sx={{
                                    ...NODE_LAYER_STYLES.typography,
                                    maxWidth: '100%', textAlign: 'center',
                                    fontStyle: !provider.model ? 'italic' : 'normal',
                                    color: provider.model ? 'text.primary' : 'text.disabled',
                                }}>
                                    {provider.model || t('rule.service.selectModel')}
                                </Typography>
                            </Box>
                        </NodeTooltip>

                        {/* Divider */}
                        <Divider sx={NODE_LAYER_STYLES.divider} />

                        {/* Row 2: provider name (center) + api style tag (right) */}
                        <Box sx={{ ...NODE_LAYER_STYLES.bottomLayer, position: 'relative', px: '28px' }}>
                            {isProviderMissing && (
                                <WarningIcon sx={{ fontSize: '1rem', color: 'warning.main', flexShrink: 0, mr: 0.5 }} />
                            )}
                            <Typography variant="body2" noWrap
                                color={isProviderMissing ? 'warning.main' : 'text.secondary'}
                                sx={{ ...NODE_LAYER_STYLES.typography, fontWeight: 400, maxWidth: '100%', textAlign: 'center' }}>
                                {providerInfo.name}
                            </Typography>
                            <Box sx={{ position: 'absolute', right: 0, top: '50%', transform: 'translateY(-50%)', display: 'flex', gap: '2px', lineHeight: 0 }}>
                                {hasDualApiStyle ? (
                                    <>
                                        <ApiStyleBadge apiStyle="openai" minimal sx={{ fontSize: '0.72rem', width: 20, height: 20 }} />
                                        <ApiStyleBadge apiStyle="anthropic" minimal sx={{ fontSize: '0.72rem', width: 20, height: 20 }} />
                                    </>
                                ) : (
                                    <ApiStyleBadge apiStyle={apiStyle} minimal sx={{ fontSize: '0.72rem', width: 20, height: 20 }} />
                                )}
                            </Box>
                        </Box>
                    </>
                )}

                {/* Action buttons (hover) */}
                <ActionButtonsBox className="action-buttons">
                    {provider.provider && providerInfo.exists && (
                        <NodeTooltip title={t('rule.service.testService')} placement="bottom">
                            <IconButton
                                size="small"
                                onClick={handleProbeClick}
                                sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                            >
                                <PlayIcon sx={{ fontSize: '1rem', color: 'success.main' }} />
                            </IconButton>
                        </NodeTooltip>
                    )}
                    <NodeTooltip title={t('rule.service.deleteService')} placement="bottom">
                        <IconButton
                            size="small"
                            onClick={handleMenuClick}
                            sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                        >
                            <DeleteIcon sx={{ fontSize: '1rem', color: 'error.main' }} />
                        </IconButton>
                    </NodeTooltip>
                </ActionButtonsBox>
            </ServiceNodeContainer>
        </ServiceNodeWrapper>
    );
};

/** @deprecated Use ServiceNode */
export const ProviderNode = ServiceNode;

export default ServiceNode;
