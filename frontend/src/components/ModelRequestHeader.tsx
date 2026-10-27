import {
    Box,
    Typography,
    Chip,
    CircularProgress,
    Button,
    IconButton,
    Menu,
    MenuItem,
    ListItemText,
    Tooltip,
    TextField,
} from '@mui/material';
import { alpha, styled, type Theme } from '@mui/material/styles';
import React, { useState } from 'react';
import {
    Edit as EditIcon,
    Check as CheckIcon,
    Close as CloseIcon,
    ExpandMore as ExpandMoreIcon,
    Extension as ExtensionIcon,
} from '@/components/icons';
import { useTranslation } from 'react-i18next';
import { isWildcardModelName } from '@/components/rule-card/utils';
import { notify } from '@/utils/notify';
import { writeToClipboard } from '@/utils/clipboard';
import { fontMono, fontSizes } from '@/theme/fonts';
import type {OpenAIEndpointSelection} from '@/hooks/useResponsesToggle';
import {CodexCompactChip} from '@/components/rule-card/CodexCompactChip';
import {CodexContextChip} from '@/components/rule-card/CodexContextChip';
import {formatTokens, resolveCodexWindow} from '@/components/rule-card/codexCompact';
import {useCodexCatalogWindow} from '@/components/rule-card/useCodexCatalogWindow';

// Styled components - compact for graph use
const HEADER_PADDING_X = 40;
const HEADER_PADDING_Y = 6;
// Fixed (not just minimum) width for the "name + edit icon" slot, so the
// controls that follow (1M, Endpoint, …) line up at the same x-position
// across every rule card. Sized to fit common model names in full (e.g.
// "deepseek-v4-flash" measures ~140px in this font) with some headroom;
// names that still don't fit truncate with an ellipsis — the hover tooltip
// shows the full name and click-to-copy still copies it in full, so an
// outlier-long name never re-breaks the alignment this exists to guarantee.
const NAME_SLOT_WIDTH = 190;

const strategyButtonSx = {
    height: 26,
    minWidth: 0,
    px: 1,
    borderRadius: 1.5,
    borderColor: 'divider',
    fontSize: fontSizes.sm,
    fontWeight: 600,
    textTransform: 'none' as const,
    whiteSpace: 'nowrap' as const,
    '&:hover': {borderColor: 'text.secondary', backgroundColor: 'action.hover'},
    '& .MuiButton-endIcon': {ml: 0.5},
};

const highlightedStrategyButtonSx = (theme: Theme) => ({
    ...strategyButtonSx,
    color: theme.palette.primary.main,
    borderColor: alpha(theme.palette.primary.main, 0.48),
    backgroundColor: alpha(theme.palette.primary.main, 0.12),
    '&:hover': {
        borderColor: theme.palette.primary.main,
        backgroundColor: alpha(theme.palette.primary.main, 0.18),
    },
});

const HeaderContainer = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'collapsible',
})<{ collapsible?: boolean }>(({ collapsible, theme }) => ({
    display: 'flex',
    // The actions stay at the right of the first row; at narrow widths it is
    // the title section that wraps (its controls drop to a second row).
    flexWrap: 'nowrap' as const,
    alignItems: 'flex-start',
    justifyContent: 'space-between',
    padding: `${HEADER_PADDING_Y}px ${HEADER_PADDING_X}px`,
    gap: 4,
    cursor: collapsible ? 'pointer' : 'default',
    ...(collapsible && {
        '&:hover': {
            backgroundColor: theme.palette.action.hover,
        },
    }),
}));

// Name + controls, then the description. One line: the description is what
// gives way (it ellipsizes), so every rule card's header lines up and the
// actions stay on the same row as the name instead of dropping under it.
const TitleSection = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-start',
    gap: theme.spacing(0.5),
    flex: '1 1 0',
    minWidth: 0,
    // Only the controls ever wrap (below); the description just shortens.
    flexWrap: 'wrap' as const,
}));

const ActionsSection = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.25),
    flexShrink: 0,
    // Stay at the right edge, level with the first row of the title.
    marginLeft: 'auto',
    minHeight: 28,
}));

const ModelNameText = styled(Typography)(({ theme }) => ({
    fontWeight: 600,
    fontSize: '0.875rem',
    color: theme.palette.text.primary,
    letterSpacing: '-0.01em',
    fontFamily: fontMono,
}));

// Main component props
export interface ModelRequestHeaderProps {
    modelName: string;
    onModelChange?: (newName: string) => void;
    editable?: boolean;
    active?: boolean;
    subtitle?: React.ReactNode;
    sx?: React.CSSProperties;
    onClick?: () => void;
    collapsible?: boolean;
    // Additional props for unified use
    extraActions?: React.ReactNode;
    isExpanded?: boolean;
    onToggleExpanded?: () => void;
    // 1M context window props
    context1M?: boolean;
    onContext1MToggle?: () => void;
    // Rule-level upstream endpoint choice. Auto follows model catalog and
    // provider declarations; Responses is checked before it is forced.
    endpointSelection?: OpenAIEndpointSelection;
    responsesProbing?: boolean;
    onEndpointSelect?: (selection: OpenAIEndpointSelection) => void;
    // Codex only: the share of the context window at which Codex auto-compacts
    // this rule's model (0 = follow Auto Config). Config-only, see CodexCompactChip.
    onCompactChange?: (percent: number) => void;
    compactPercent?: number;
    // Codex only: the context window written into the catalog for this rule's
    // model (0 = automatic). Config-only, see CodexContextChip.
    onContextWindowChange?: (window: number) => void;
    contextWindow?: number;
    // False when none of the rule's services is OpenAI-compatible: the chip stays
    // (and works) but its tooltip says it has no effect there.
    endpointApplicable?: boolean;
    // Plugins are low-frequency: a "Plugins N" tag shows only when the rule
    // has some; adding / removing lives in the rule's "⋯" menu.
    pluginsCount?: number;
    onPluginsClick?: (anchor: HTMLElement) => void;
}

export const ModelRequestHeader: React.FC<ModelRequestHeaderProps> = ({
    modelName,
    onModelChange,
    editable = false,
    active = true,
    subtitle,
    sx,
    onClick,
    collapsible = false,
    extraActions,
    isExpanded = true,
    onToggleExpanded,
    context1M = false,
    onContext1MToggle,
    endpointSelection = 'auto',
    responsesProbing = false,
    onEndpointSelect,
    onCompactChange,
    compactPercent = 0,
    onContextWindowChange,
    contextWindow = 0,
    endpointApplicable = true,
    pluginsCount = 0,
    onPluginsClick,
}) => {
    const { t } = useTranslation();
    const [editMode, setEditMode] = useState(false);
    const [tempValue, setTempValue] = useState(modelName);
    const [contextMenuAnchor, setContextMenuAnchor] = useState<HTMLElement | null>(null);
    const [endpointMenuAnchor, setEndpointMenuAnchor] = useState<HTMLElement | null>(null);

    React.useEffect(() => {
        setTempValue(modelName);
    }, [modelName]);

    // On a Codex rule (it has the Context chip) the 1M menu says the window
    // each choice writes into Codex's catalog: the routed model's larger
    // window can be 872K or 1M, or there may be none.
    const codexWindows = useCodexCatalogWindow(modelName, Boolean(contextMenuAnchor) && Boolean(onContextWindowChange));
    const codexWindowLine = (on: boolean): string | null => {
        if (!onContextWindowChange) return null;
        const w = resolveCodexWindow(contextWindow, on ? codexWindows.on : codexWindows.off);
        if (w.source === 'rule') return t('ruleHeader.context1M.codexWindowPinned', { value: formatTokens(w.window) });
        if (codexWindows.status === 'unavailable') return null;
        if (codexWindows.status === 'pending') return t('ruleHeader.context1M.codexWindow', { value: '…' });
        return t(w.source === 'default' ? 'ruleHeader.context1M.codexWindowGuess' : 'ruleHeader.context1M.codexWindow', { value: formatTokens(w.window) });
    };
    const context1MSecondary = (text: string, on: boolean) => {
        const line = codexWindowLine(on);
        if (!line) return text;
        return (
            <>
                {text}
                <Box component="span" data-codex-window={on ? 'on' : 'off'} sx={{display: 'block', mt: 0.25, color: 'text.primary', fontVariantNumeric: 'tabular-nums'}}>
                    {line}
                </Box>
            </>
        );
    };

    const handleSave = () => {
        if (onModelChange && tempValue.trim()) {
            onModelChange(tempValue.trim());
        }
        setEditMode(false);
    };

    const handleCancel = () => {
        setTempValue(modelName);
        setEditMode(false);
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter') {
            handleSave();
        } else if (e.key === 'Escape') {
            handleCancel();
        }
    };

    const handleCopy = async () => {
        try {
            await writeToClipboard(modelName);
            notify.success(`Model name "${modelName}" copied to clipboard`);
        } catch {
            notify.error('Failed to copy to clipboard');
        }
    };

    const isWildcard = isWildcardModelName(modelName);

    const renderTitle = () => {
        if (editMode && editable) {
            return (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, width: '100%', maxWidth: 300 }} onClick={(e) => e.stopPropagation()}>
                    <TextField
                        value={tempValue}
                        onChange={(e) => setTempValue(e.target.value)}
                        onBlur={handleSave}
                        onKeyDown={handleKeyDown}
                        size="small"
                        fullWidth
                        placeholder="Model name..."
                        error={!tempValue.trim()}
                        autoFocus
                        sx={{
                            '& .MuiInputBase-input': {
                                color: 'text.primary',
                                fontWeight: 600,
                                fontSize: '0.875rem',
                                px: 0.5,
                            },
                            '& .MuiOutlinedInput-notchedOutline': {
                                borderColor: 'divider',
                            },
                            '& .MuiInputBase-root': {
                                padding: '2px 8px',
                            },
                        }}
                    />
                    <Tooltip title={tempValue.trim() ? 'Save (Enter)' : 'Model name cannot be empty'}>
                        <span>
                            <IconButton size="small" onClick={(e) => { e.stopPropagation(); handleSave(); }} disabled={!tempValue.trim()} sx={{ p: 0.5 }}>
                                <CheckIcon sx={{ fontSize: '1rem' }} />
                            </IconButton>
                        </span>
                    </Tooltip>
                    <Tooltip title="Cancel (Esc)">
                        <IconButton size="small" onClick={(e) => { e.stopPropagation(); handleCancel(); }} sx={{ p: 0.5 }}>
                            <CloseIcon sx={{ fontSize: '1rem' }} />
                        </IconButton>
                    </Tooltip>
                </Box>
            );
        }

        return (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0, flexWrap: 'wrap', rowGap: 0.5 }} onClick={(e) => e.stopPropagation()}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, width: NAME_SLOT_WIDTH, flexShrink: 0 }}>
                    <Tooltip
                        title={modelName ? `${modelName} (click to copy)` : 'No model specified'}
                        placement="top"
                    >
                        {isWildcard ? (
                            <Chip
                                label={
                                    <Typography variant="body2" sx={{ fontWeight: 600, fontSize: fontSizes.md }}>
                                        {modelName}
                                    </Typography>
                                }
                                size="small"
                                variant="outlined"
                                onClick={(e) => { e.stopPropagation(); handleCopy(); }}
                                sx={{
                                    '& .MuiChip-label': { fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis' },
                                    height: 22,
                                    minWidth: 0,
                                    flex: '0 1 auto',
                                    cursor: 'pointer',
                                    '&:hover': {
                                        backgroundColor: 'action.hover',
                                    },
                                }}
                            />
                        ) : (
                            <ModelNameText
                                onClick={(e) => { e.stopPropagation(); handleCopy(); }}
                                sx={{
                                    minWidth: 0,
                                    flex: '0 1 auto',
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    whiteSpace: 'nowrap',
                                    cursor: modelName ? 'pointer' : 'default',
                                    '&:hover': modelName ? {
                                        textDecoration: 'underline',
                                        textDecorationStyle: 'dotted',
                                        textDecorationColor: 'text.secondary',
                                        color: 'primary.main',
                                    } : {},
                                }}
                            >
                                {modelName}
                            </ModelNameText>
                        )}
                    </Tooltip>
                    <Tooltip title="Edit model name — this is what's matched against incoming API calls. Supports wildcards (* or [any]) to match any model.">
                        <IconButton
                            size="small"
                            onClick={(e) => { e.stopPropagation(); setEditMode(true); }}
                            sx={{
                                opacity: editable ? 0.6 : 0,
                                p: 0.5,
                                ml: 0.25,
                                flexShrink: 0,
                                pointerEvents: editable ? 'auto' : 'none',
                                '&:hover': { opacity: 1 }
                            }}
                        >
                            <EditIcon sx={{ fontSize: '0.95rem' }} />
                        </IconButton>
                    </Tooltip>
                </Box>

                {/* The rule's chips (1M, Endpoint, Plugins) move as one group: when the
                    row is too narrow they drop to a line of their own, together. */}
                <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'nowrap' }}>
                {/* Keep 1M visible; Off only means this rule does not add 1M. */}
                {onContext1MToggle && (
                    <Box sx={{ml: 0.5}} onClick={(e) => e.stopPropagation()}>
                        <Tooltip title="Choose whether this rule adds 1M context">
                            <span>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    color={context1M ? 'primary' : 'inherit'}
                                    endIcon={<ExpandMoreIcon sx={{fontSize: 14}} />}
                                    disabled={!active}
                                    aria-label={`1M context: ${context1M ? 'On' : 'Off'}`}
                                    aria-haspopup="menu"
                                    aria-expanded={Boolean(contextMenuAnchor)}
                                    onClick={(e) => setContextMenuAnchor(e.currentTarget)}
                                    sx={context1M ? highlightedStrategyButtonSx : strategyButtonSx}
                                >
                                    1M: {context1M ? 'On' : 'Off'}
                                </Button>
                            </span>
                        </Tooltip>
                        <Menu
                            anchorEl={contextMenuAnchor}
                            open={Boolean(contextMenuAnchor)}
                            onClose={() => setContextMenuAnchor(null)}
                            onClick={(e) => e.stopPropagation()}
                            slotProps={{paper: {sx: {maxWidth: 320}}}}
                        >
                            <MenuItem
                                selected={!context1M}
                                onClick={() => {
                                    setContextMenuAnchor(null);
                                    if (context1M) onContext1MToggle();
                                }}
                                sx={{whiteSpace: 'normal', alignItems: 'flex-start', py: 0.75}}
                            >
                                <ListItemText primary={t('ruleHeader.context1M.off')} secondary={context1MSecondary(t('ruleHeader.context1M.offDescription'), false)} slotProps={{secondary: {sx: {whiteSpace: 'normal', lineHeight: 1.3}}}} />
                            </MenuItem>
                            <MenuItem
                                selected={context1M}
                                onClick={() => {
                                    setContextMenuAnchor(null);
                                    if (!context1M) onContext1MToggle();
                                }}
                                sx={{whiteSpace: 'normal', alignItems: 'flex-start', py: 0.75}}
                            >
                                <ListItemText primary={t('ruleHeader.context1M.on')} secondary={context1MSecondary(t('ruleHeader.context1M.onDescription'), true)} slotProps={{secondary: {sx: {whiteSpace: 'normal', lineHeight: 1.3}}}} />
                            </MenuItem>
                        </Menu>
                    </Box>
                )}

                {onEndpointSelect && (
                    <Box sx={{ml: 0.5}} onClick={(e) => e.stopPropagation()}>
                        <Tooltip title={endpointApplicable ? 'Choose the upstream endpoint for this rule' : t('ruleHeader.endpointNotApplicable')}>
                            <span>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    color={endpointSelection === 'auto' ? 'inherit' : 'primary'}
                                    endIcon={responsesProbing ? <CircularProgress size={12} /> : <ExpandMoreIcon sx={{fontSize: 14}} />}
                                    disabled={!active || responsesProbing}
                                    aria-label={`Upstream endpoint: ${endpointSelection}`}
                                    aria-haspopup="menu"
                                    aria-expanded={Boolean(endpointMenuAnchor)}
                                    onClick={(e) => setEndpointMenuAnchor(e.currentTarget)}
                                    sx={endpointSelection === 'auto' ? strategyButtonSx : highlightedStrategyButtonSx}
                                >
                                    Endpoint: {endpointSelection === 'auto' ? 'Auto' : endpointSelection === 'chat' ? 'Chat' : endpointSelection === 'anthropic' ? 'Anthropic' : 'Responses'}
                                </Button>
                            </span>
                        </Tooltip>
                        <Menu
                            anchorEl={endpointMenuAnchor}
                            open={Boolean(endpointMenuAnchor)}
                            onClose={() => setEndpointMenuAnchor(null)}
                            onClick={(e) => e.stopPropagation()}
                            slotProps={{paper: {sx: {maxWidth: 320}}}}
                        >
                            {([
                                ['auto', 'Auto', 'Use the model Catalog, then the provider setting; default to Chat. When both are supported, follow the client request.'],
                                ['chat', 'Chat', 'Always send Chat Completions to the upstream provider.'],
                                ['responses', 'Responses', 'Always send Responses to the upstream provider. Support is checked before saving.'],
                                ['anthropic', 'Anthropic', 'Always send Anthropic Messages to the upstream provider, converting OpenAI clients. Providers without an Anthropic endpoint fall back to Auto.'],
                            ] as const).map(([value, label, description]) => (
                                <MenuItem
                                    key={value}
                                    selected={endpointSelection === value}
                                    onClick={() => {
                                        setEndpointMenuAnchor(null);
                                        onEndpointSelect(value);
                                    }}
                                    sx={{whiteSpace: 'normal', alignItems: 'flex-start', py: 0.75}}
                                >
                                    <ListItemText
                                        primary={label}
                                        secondary={description}
                                        slotProps={{secondary: {sx: {whiteSpace: 'normal', lineHeight: 1.3}}}}
                                    />
                                </MenuItem>
                            ))}
                        </Menu>
                    </Box>
                )}

                {onContextWindowChange && (
                    <CodexContextChip
                        window={contextWindow}
                        context1M={context1M}
                        requestModel={modelName}
                        disabled={!active}
                        onChange={onContextWindowChange}
                        buttonSx={strategyButtonSx}
                        highlightedButtonSx={highlightedStrategyButtonSx}
                    />
                )}

                {onCompactChange && (
                    <CodexCompactChip
                        percent={compactPercent}
                        contextWindow={contextWindow}
                        context1M={context1M}
                        requestModel={modelName}
                        disabled={!active}
                        onChange={onCompactChange}
                        buttonSx={strategyButtonSx}
                        highlightedButtonSx={highlightedStrategyButtonSx}
                    />
                )}

                {pluginsCount > 0 && onPluginsClick && (
                    <Box sx={{ml: 0.5}} onClick={(e) => e.stopPropagation()}>
                        <Tooltip title={t('ruleHeader.pluginsTooltip')}>
                            <Button
                                size="small"
                                variant="outlined"
                                color="inherit"
                                startIcon={<ExtensionIcon sx={{fontSize: 14}} />}
                                aria-label={`${t('ruleHeader.plugins')} ${pluginsCount}`}
                                aria-haspopup="dialog"
                                onClick={(e) => onPluginsClick(e.currentTarget)}
                                sx={{...strategyButtonSx, '& .MuiButton-startIcon': {mr: 0.5, ml: 0}}}
                            >
                                {t('ruleHeader.plugins')} {pluginsCount}
                            </Button>
                        </Tooltip>
                    </Box>
                )}
                </Box>
            </Box>
        );
    };

    const renderActions = () => {
        return (
            <ActionsSection>
                {/* Extra Actions (from parent) */}
                {extraActions && (
                    <Box onClick={(e) => e.stopPropagation()}>
                        {extraActions}
                    </Box>
                )}


                {/* Expand/Collapse Button */}
                {collapsible && onToggleExpanded && (
                    <Tooltip title={isExpanded ? 'Collapse' : 'Expand'}>
                        <ExpandMoreIcon
                            onClick={(e) => {
                                e.stopPropagation();
                                onToggleExpanded();
                            }}
                            sx={{
                                cursor: 'pointer',
                                transition: 'transform 0.2s',
                                transform: isExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
                                fontSize: '1.1rem',
                                color: 'text.secondary',
                                '&:hover': { color: 'text.primary' },
                            }}
                        />
                    </Tooltip>
                )}
            </ActionsSection>
        );
    };

    return (
        <HeaderContainer
            collapsible={collapsible}
            sx={sx}
            onClick={onClick}
        >
            <TitleSection>
                {renderTitle()}
                {subtitle && !editMode && (
                    <Typography
                        variant="caption"
                        title={typeof subtitle === 'string' ? subtitle : undefined}
                        sx={{
                            color: 'text.secondary', ml: 1, flex: '1 1 120px', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                            // Less than ~120px left beside the controls: the description drops
                            // to its own line (full width) instead of showing as "R…".
                            // Its text length itself never counts toward the width.
                            contain: 'inline-size',
                        }}
                    >
                        {subtitle}
                    </Typography>
                )}
            </TitleSection>

            {renderActions()}
        </HeaderContainer>
    );
};

export default ModelRequestHeader;
