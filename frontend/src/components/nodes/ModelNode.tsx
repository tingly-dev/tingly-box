import {
    Box,
    TextField,
    Typography,
    Chip,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Tooltip,
    ToggleButton,
    Divider,
} from '@mui/material';
import { Settings as SettingsIcon } from '@mui/icons-material';
import { styled } from '@mui/material/styles';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { StyledModelNode, NODE_LAYER_STYLES } from './styles.tsx';
import { isWildcardModelName } from '@/components/rule-card/utils';

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

const ModelNodeWrapper = styled(Box)(({ theme }) => ({
    position: 'relative',
    '&:hover .action-buttons': {
        opacity: 1,
    }
}));

// Model Node Component Props
export interface ModelNodeProps {
    active: boolean;
    label: string;
    value: string;
    editable?: boolean;
    onUpdate?: (value: string) => void;
    showStatusIcon?: boolean;
    compact?: boolean;
    // Smart routing props
    smartEnabled?: boolean;
    showSmartSwitch?: boolean;
    switchDisabled?: boolean;
    onSwitch?: () => void;
}

export const ModelNode: React.FC<ModelNodeProps> = ({
    active,
    label,
    value,
    editable = false,
    onUpdate,
    showStatusIcon = true,
    compact = false,
    // Smart routing props
    smartEnabled = false,
    showSmartSwitch = false,
    switchDisabled = false,
    onSwitch,
}) => {
    const { t } = useTranslation();
    const [editMode, setEditMode] = useState(false);
    const [tempValue, setTempValue] = useState(value);
    const [anchorEl, setAnchorEl] = React.useState<null | HTMLElement>(null);
    const menuOpen = Boolean(anchorEl);

    const isWildcard = isWildcardModelName(value);

    React.useEffect(() => {
        setTempValue(value);
    }, [value]);

    const handleSave = () => {
        if (onUpdate && tempValue.trim()) {
            onUpdate(tempValue.trim());
        }
        setEditMode(false);
    };

    const handleCancel = () => {
        setTempValue(value);
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
        setAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        setAnchorEl(null);
    };

    const handleSetWildcard = () => {
        handleMenuClose();
        if (onUpdate) {
            onUpdate('[any]');
        }
    };

    const handleSetCustom = () => {
        handleMenuClose();
        setEditMode(true);
    };

    return (
        <ModelNodeWrapper>
            <Menu
                anchorEl={anchorEl}
                open={menuOpen}
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
                <MenuItem onClick={handleSetCustom}>
                    <ListItemText sx={{ fontWeight: !isWildcard ? 600 : 400 }}>
                        Custom model name
                    </ListItemText>
                </MenuItem>
                <MenuItem onClick={handleMenuClose} sx={{ color: 'text.secondary' }}>
                    <ListItemText>Cancel</ListItemText>
                </MenuItem>
            </Menu>
            <StyledModelNode compact={compact} sx={{ display: 'flex', flexDirection: 'column' }}>
                {/* Top Layer - Model Name */}
                <Box sx={NODE_LAYER_STYLES.topLayer}>
                    {editMode && editable ? (
                        <Box sx={{ display: 'flex', alignItems: 'center', width: '100%', p: 1 }}>
                        <TextField
                            value={tempValue}
                            onChange={(e) => setTempValue(e.target.value)}
                            onBlur={handleSave}
                            onKeyDown={handleKeyDown}
                            size="small"
                            fullWidth
                            placeholder="Enter custom model name..."
                            sx={{
                                '& .MuiInputBase-input': {
                                    color: 'text.primary',
                                    fontWeight: 'inherit',
                                    fontSize: 'inherit',
                                    backgroundColor: 'transparent',
                                },
                                '& .MuiOutlinedInput-notchedOutline': {
                                    borderColor: 'text.secondary',
                                },
                                '& .MuiOutlinedInput-root:hover .MuiOutlinedInput-notchedOutline': {
                                    borderColor: 'text.primary',
                                },
                            }}
                        />
                    </Box>
                    ) : (
                    <Box
                        onClick={() => editable && setEditMode(true)}
                        sx={{
                            cursor: editable ? 'pointer' : 'default',
                            width: '100%',
                            height: '100%',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            '&:hover': editable ? {
                                backgroundColor: 'action.hover',
                                borderRadius: 1,
                            } : {},
                        }}
                    >
                        {isWildcard ? (
                            <Tooltip title="Matches any model (wildcard)">
                                <Chip
                                    label={
                                        <Typography variant="body2" sx={{ fontWeight: 600, fontSize: '0.9rem' }}>
                                            {value}
                                        </Typography>
                                    }
                                    size="small"
                                    variant="outlined"
                                    sx={{
                                        '& .MuiChip-label': {
                                            fontWeight: 600,
                                        },
                                    }}
                                />
                            </Tooltip>
                        ) : (
                            <Typography variant="body2" sx={{ ...NODE_LAYER_STYLES.typography, color: 'text.primary' }}>
                                {value || label}
                            </Typography>
                        )}
                    </Box>
                    )}
                </Box>

                {/* Divider - only show when smart switch is visible */}
                {showSmartSwitch && !editMode && (
                    <Divider sx={NODE_LAYER_STYLES.divider} />
                )}

                {/* Bottom Layer - Smart Switch */}
                {showSmartSwitch && !editMode && (
                    <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                        <Tooltip title="Direct routing mode" arrow>
                            <ToggleButton
                                value="direct"
                                selected={!smartEnabled}
                                disabled={!active || switchDisabled}
                                onClick={onSwitch}
                                sx={{
                                    ...NODE_LAYER_STYLES.toggleButton,
                                    flex: 1,
                                    borderColor: 'text.primary',
                                    '&.Mui-selected': {
                                        backgroundColor: !smartEnabled ? 'secondary.main' : 'transparent',
                                        color: !smartEnabled ? 'white' : 'text.primary',
                                        borderColor: 'text.primary',
                                    },
                                    '&:hover': {
                                        backgroundColor: !smartEnabled ? 'secondary.dark' : 'action.hover',
                                    },
                                }}
                            >
                                Direct
                            </ToggleButton>
                        </Tooltip>
                        <Tooltip title="Smart routing mode" arrow>
                            <ToggleButton
                                value="smart"
                                selected={smartEnabled}
                                disabled={!active || switchDisabled}
                                onClick={onSwitch}
                                sx={{
                                    ...NODE_LAYER_STYLES.toggleButton,
                                    flex: 1,
                                    borderColor: 'text.primary',
                                    '&.Mui-selected': {
                                        backgroundColor: smartEnabled ? 'secondary.main' : 'transparent',
                                        color: smartEnabled ? 'white' : 'text.primary',
                                        borderColor: 'text.primary',
                                    },
                                    '&:hover': {
                                        backgroundColor: smartEnabled ? 'secondary.dark' : 'action.hover',
                                    },
                                }}
                            >
                                Smart
                            </ToggleButton>
                        </Tooltip>
                    </Box>
                )}
            </StyledModelNode>
            {/* Action Buttons - visible on hover */}
            {editable && (
                <ActionButtonsBox className="action-buttons">
                    <Tooltip title="Change model">
                        <IconButton
                            size="small"
                            onClick={handleMenuClick}
                            sx={{ p: 0.5, backgroundColor: 'background.paper' }}
                        >
                            <SettingsIcon sx={{ fontSize: '1rem', color: 'text.primary' }} />
                        </IconButton>
                    </Tooltip>
                </ActionButtonsBox>
            )}
        </ModelNodeWrapper>
    );
};
