import {
    Box,
    Typography,
    Chip,
    IconButton,
    Tooltip,
    TextField,
    Menu,
    MenuItem,
    ListItemText,
} from '@mui/material';
import { alpha, styled } from '@mui/material/styles';
import React, { useState } from 'react';
import {
    Edit as EditIcon,
    Check as CheckIcon,
    Close as CloseIcon,
    ExpandMore as ExpandMoreIcon,
    HelpOutline as HelpOutlineIcon,
} from '@/components/icons';
import { isWildcardModelName } from '@/components/rule-card/utils';
import { notify } from '@/utils/notify';
import { fontMono } from '@/theme/fonts';

// Styled components - compact for graph use
const HEADER_PADDING_X = 40;
const HEADER_PADDING_Y = 6;

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
    // When provided, renders a persistent "?" affordance that opens the routing
    // guide — makes the node-level education discoverable for first-time users.
    onShowGuide?: () => void;
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
    onShowGuide,
}) => {
    const [editMode, setEditMode] = useState(false);
    const [tempValue, setTempValue] = useState(modelName);
    const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);

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

    const handleMenuClose = () => {
        setMenuAnchor(null);
    };

    const handleSetWildcard = () => {
        handleMenuClose();
        if (onModelChange) {
            onModelChange('[any]');
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
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }} onClick={(e) => e.stopPropagation()}>
                <Tooltip
                    title={modelName
                        ? `The model name that clients use to make requests. This will be matched against incoming API calls. Supports wildcards (* or [any]) for matching any model. (click to copy)`
                        : 'No model specified'}
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
                                '& .MuiChip-label': { fontWeight: 600 },
                                height: 22,
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
                <Tooltip title="Edit model name">
                    <IconButton
                        size="small"
                        onClick={(e) => { e.stopPropagation(); setEditMode(true); }}
                        sx={{
                            opacity: editable ? 0.6 : 0,
                            p: 0.5,
                            ml: 0.25,
                            pointerEvents: editable ? 'auto' : 'none',
                            '&:hover': { opacity: 1 }
                        }}
                    >
                        <EditIcon sx={{ fontSize: '0.95rem' }} />
                    </IconButton>
                </Tooltip>
            </Box>
        );
    };

    const renderActions = () => {
        return (
            <ActionsSection>
                {/* Guide button — always-visible entry into the routing walkthrough */}
                {onShowGuide && (
                    <Tooltip title="How routing works">
                        <IconButton
                            size="small"
                            aria-label="How routing works"
                            onClick={(e) => { e.stopPropagation(); onShowGuide(); }}
                            sx={{
                                p: 0.5,
                                color: 'text.secondary',
                                '&:hover': { color: 'primary.main' },
                            }}
                        >
                            <HelpOutlineIcon sx={{ fontSize: '1.05rem' }} />
                        </IconButton>
                    </Tooltip>
                )}

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

            {/* Context Menu for Model Name */}
            <Menu
                anchorEl={menuAnchor}
                open={Boolean(menuAnchor)}
                onClose={handleMenuClose}
                onClick={(e) => e.stopPropagation()}
                transformOrigin={{ horizontal: 'right', vertical: 'top' }}
                anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
            >
                <MenuItem onClick={handleSetWildcard}>
                    <ListItemText sx={{ fontWeight: isWildcard ? 600 : 400 }}>
                        Match any model (* or [any])
                    </ListItemText>
                </MenuItem>
                <MenuItem onClick={() => { handleMenuClose(); setEditMode(true); }}>
                    <ListItemText sx={{ fontWeight: !isWildcard ? 600 : 400 }}>
                        Custom model name
                    </ListItemText>
                </MenuItem>
                <MenuItem onClick={handleMenuClose} sx={{ color: 'text.secondary' }}>
                    <ListItemText>Cancel</ListItemText>
                </MenuItem>
            </Menu>
        </HeaderContainer>
    );
};

export default ModelRequestHeader;
