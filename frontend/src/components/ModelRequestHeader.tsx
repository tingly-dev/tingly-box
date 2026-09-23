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
} from '@/components/icons';
import { isWildcardModelName } from '@/components/rule-card/utils';
import { notify } from '@/utils/notify';
import { fontMono } from '@/theme/fonts';
import type {OpenAIEndpointSelection} from '@/hooks/useResponsesToggle';

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
    fontSize: '0.75rem',
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
    flexWrap: 'wrap' as const,
    alignItems: 'center',
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

const TitleSection = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: theme.spacing(0.5),
    flexGrow: 0,
    minWidth: 0,
    flexWrap: 'wrap' as const,
}));

const ActionsSection = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.25),
    flexShrink: 0,
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
}) => {
    const [editMode, setEditMode] = useState(false);
    const [tempValue, setTempValue] = useState(modelName);
    const [contextMenuAnchor, setContextMenuAnchor] = useState<HTMLElement | null>(null);
    const [endpointMenuAnchor, setEndpointMenuAnchor] = useState<HTMLElement | null>(null);

    React.useEffect(() => {
        setTempValue(modelName);
    }, [modelName]);

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

    const handleCopy = () => {
        void navigator.clipboard.writeText(modelName);
        notify.success(`Model name "${modelName}" copied to clipboard`);
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
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }} onClick={(e) => e.stopPropagation()}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, width: NAME_SLOT_WIDTH, flexShrink: 0 }}>
                    <Tooltip
                        title={modelName ? `${modelName} (click to copy)` : 'No model specified'}
                        placement="top"
                    >
                        {isWildcard ? (
                            <Chip
                                label={
                                    <Typography variant="body2" sx={{ fontWeight: 600, fontSize: '0.8rem' }}>
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
                                <ListItemText primary="Off" secondary="Do not add 1M for this rule. Client-requested 1M still works where supported." slotProps={{secondary: {sx: {whiteSpace: 'normal', lineHeight: 1.3}}}} />
                            </MenuItem>
                            <MenuItem
                                selected={context1M}
                                onClick={() => {
                                    setContextMenuAnchor(null);
                                    if (!context1M) onContext1MToggle();
                                }}
                                sx={{whiteSpace: 'normal', alignItems: 'flex-start', py: 0.75}}
                            >
                                <ListItemText primary="On" secondary="Add 1M for this rule. The upstream model must support it." slotProps={{secondary: {sx: {whiteSpace: 'normal', lineHeight: 1.3}}}} />
                            </MenuItem>
                        </Menu>
                    </Box>
                )}

                {onEndpointSelect && (
                    <Box sx={{ml: 0.5}} onClick={(e) => e.stopPropagation()}>
                        <Tooltip title="Choose the upstream OpenAI endpoint for this rule">
                            <span>
                                <Button
                                    size="small"
                                    variant="outlined"
                                    color={endpointSelection === 'auto' ? 'inherit' : 'primary'}
                                    endIcon={responsesProbing ? <CircularProgress size={12} /> : <ExpandMoreIcon sx={{fontSize: 14}} />}
                                    disabled={!active || responsesProbing}
                                    aria-label={`OpenAI endpoint: ${endpointSelection}`}
                                    aria-haspopup="menu"
                                    aria-expanded={Boolean(endpointMenuAnchor)}
                                    onClick={(e) => setEndpointMenuAnchor(e.currentTarget)}
                                    sx={endpointSelection === 'auto' ? strategyButtonSx : highlightedStrategyButtonSx}
                                >
                                    Endpoint: {endpointSelection === 'auto' ? 'Auto' : endpointSelection === 'chat' ? 'Chat' : 'Responses'}
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
                    <Typography variant="caption" sx={{ color: 'text.secondary', ml: 1 }}>
                        {subtitle}
                    </Typography>
                )}
            </TitleSection>

            {renderActions()}
        </HeaderContainer>
    );
};

export default ModelRequestHeader;
