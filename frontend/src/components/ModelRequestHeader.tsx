import {
    Box,
    Typography,
    Chip,
    IconButton,
    Tooltip,
    TextField,
    Menu,
    MenuItem,
    ListItemIcon,
    ListItemText,
    ToggleButton,
} from '@mui/material';
import { alpha, styled } from '@mui/material/styles';
import React, { useState } from 'react';
import {
    Edit as EditIcon,
    Check as CheckIcon,
    Close as CloseIcon,
    ExpandMore as ExpandMoreIcon,
} from '@/components/icons';
import {
    getRouteGraphActiveColor,
    getRouteGraphControlFill,
    getRouteGraphControlFillHover,
    NODE_LAYER_STYLES,
} from '@/components/nodes/styles';
import { isWildcardModelName } from '@/components/rule-card/utils';

// Styled components - compact for graph use
const HeaderContainer = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'collapsible',
})<{ collapsible?: boolean }>(({ collapsible }) => ({
    display: 'flex',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: `2px 8px`,  // Even more compact
    gap: 4,
    cursor: collapsible ? 'pointer' : 'default',
    ...(collapsible && {
        '&:hover': {
            backgroundColor: 'action.hover',
        },
    }),
}));

const TitleSection = styled(Box)(({ theme }) => ({
    display: 'flex',
    alignItems: 'center',
    gap: theme.spacing(0.5),
    flexGrow: 1,
    minWidth: 0,
    flexWrap: 'wrap',
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
    fontFamily: 'monospace',
}));

const StyledChip = styled(Chip)(({ theme }) => ({
    height: 28,
    borderRadius: 14,
    fontSize: '0.75rem',
    fontWeight: 500,
    px: 1,
    backgroundColor: theme.palette.mode === 'dark'
        ? alpha(getRouteGraphActiveColor(theme), 0.12)
        : alpha(getRouteGraphActiveColor(theme), 0.08),
    color: getRouteGraphActiveColor(theme),
    border: `1px solid ${alpha(getRouteGraphActiveColor(theme), 0.3)}`,
    '&:hover': {
        backgroundColor: alpha(getRouteGraphActiveColor(theme), 0.15),
    },
    transition: 'all 0.16s ease',
}));

const ModeToggleChip = styled(ToggleButton, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    ...NODE_LAYER_STYLES.toggleButton,
    flex: 1,
    height: 32,
    borderColor: alpha(getRouteGraphActiveColor(theme), 0.7),
    color: active ? theme.palette.common.white : theme.palette.text.secondary,
    '&.Mui-selected': {
        backgroundColor: active ? getRouteGraphControlFill(theme) : 'transparent',
        color: active ? theme.palette.common.white : theme.palette.text.primary,
        borderColor: active ? getRouteGraphControlFill(theme) : getRouteGraphActiveColor(theme),
        '& .MuiSvgIcon-root': {
            color: theme.palette.common.white,
        },
        '&:hover': {
            backgroundColor: getRouteGraphControlFillHover(theme),
        },
    },
    '&:hover': {
        backgroundColor: active
            ? getRouteGraphControlFillHover(theme)
            : alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.16 : 0.08),
    },
}));

// Action button types
export interface ModelRequestHeaderAction {
    id: string;
    icon: React.ReactNode;
    label: string;
    onClick: () => void;
    disabled?: boolean;
    variant?: 'chip' | 'icon-button';
    showInEdit?: boolean;
}

// Main component props
export interface ModelRequestHeaderProps {
    modelName: string;
    onModelChange?: (newName: string) => void;
    editable?: boolean;
    active?: boolean;
    smartEnabled?: boolean;
    onSmartModeToggle?: () => void;
    actions?: ModelRequestHeaderAction[];
    subtitle?: React.ReactNode;
    responseModelName?: string;  // For response model transformation
    sx?: React.CSSProperties;
    onClick?: () => void;
    collapsible?: boolean;
    // Additional props for unified use
    extraActions?: React.ReactNode;
    isExpanded?: boolean;
    onToggleExpanded?: () => void;
}

export const ModelRequestHeader: React.FC<ModelRequestHeaderProps> = ({
    modelName,
    onModelChange,
    editable = false,
    active = true,
    smartEnabled = false,
    onSmartModeToggle,
    actions = [],
    subtitle,
    responseModelName,
    sx,
    onClick,
    collapsible = false,
    extraActions,
    isExpanded = true,
    onToggleExpanded,
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

    const handleMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setMenuAnchor(event.currentTarget);
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

    // Filter actions based on edit mode
    const visibleActions = actions.filter(
        action => editMode ? action.showInEdit !== false : action.showInEdit !== true
    );

    const renderTitle = () => {
        if (editMode && editable) {
            return (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, width: '100%', maxWidth: 300 }}>
                    <TextField
                        value={tempValue}
                        onChange={(e) => setTempValue(e.target.value)}
                        onBlur={handleSave}
                        onKeyDown={handleKeyDown}
                        size="small"
                        fullWidth
                        placeholder="Model name..."
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
                    <Tooltip title="Save (Enter)">
                        <IconButton size="small" onClick={handleSave} sx={{ p: 0.5 }}>
                            <CheckIcon sx={{ fontSize: '1rem' }} />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Cancel (Esc)">
                        <IconButton size="small" onClick={handleCancel} sx={{ p: 0.5 }}>
                            <CloseIcon sx={{ fontSize: '1rem' }} />
                        </IconButton>
                    </Tooltip>
                </Box>
            );
        }

        return (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, minWidth: 0 }}>
                {isWildcard ? (
                    <Chip
                        label={
                            <Typography variant="body2" sx={{ fontWeight: 600, fontSize: '0.8rem' }}>
                                {modelName}
                            </Typography>
                        }
                        size="small"
                        variant="outlined"
                        sx={{
                            '& .MuiChip-label': { fontWeight: 600 },
                            height: 22,
                        }}
                    />
                ) : (
                    <ModelNameText
                        onClick={() => editable && setEditMode(true)}
                        sx={{
                            cursor: editable ? 'pointer' : 'default',
                            ...(editable && {
                                '&:hover': {
                                    textDecoration: 'underline',
                                    textDecorationStyle: 'dotted',
                                    textDecorationColor: 'text.secondary',
                                },
                            }),
                        }}
                    >
                        {modelName}
                    </ModelNameText>
                )}
                {editable && !editMode && (
                    <Tooltip title="Edit model name">
                        <IconButton
                            size="small"
                            onClick={() => setEditMode(true)}
                            sx={{
                                opacity: 0.6,
                                p: 0.5,
                                '&:hover': { opacity: 1 }
                            }}
                        >
                            <EditIcon sx={{ fontSize: '1rem' }} />
                        </IconButton>
                    </Tooltip>
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

                {/* Response Model Chip */}
                {responseModelName && (
                    <Tooltip title={`Response model: ${responseModelName}`}>
                        <Chip
                            label={`→ ${responseModelName}`}
                            size="small"
                            color="info"
                            onClick={(e) => e.stopPropagation()}
                            sx={{
                                opacity: active ? 1 : 0.5,
                                backgroundColor: active ? 'info.main' : 'action.disabled',
                                color: active ? 'info.contrastText' : 'text.disabled',
                                height: 22,
                                fontSize: '0.7rem',
                            }}
                        />
                    </Tooltip>
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
