import { ToggleButton } from '@mui/material';
import { alpha, styled } from '@mui/material/styles';
import React from 'react';
import {
    getRouteGraphActiveColor,
    getRouteGraphControlFill,
    getRouteGraphControlFillHover,
    NODE_LAYER_STYLES,
    SMALL_NODE_STYLES,
} from './styles';
import {
    AutoAwesome as AutoAwesomeIcon,
    NearMeOutlined as DirectIcon,
} from '@/components/icons';

// Styled EntryNode matching ActionAddNode style
const StyledEntryNode = styled('div')<{ active: boolean; compact?: boolean }>(
    ({ active, compact, theme }) => ({
        display: 'flex',
        flexDirection: compact ? 'row' : 'column',
        justifyContent: 'center',
        alignItems: 'center',
        gap: compact ? 4 : 4,
        width: compact ? 120 : SMALL_NODE_STYLES.width,
        height: compact ? 32 : SMALL_NODE_STYLES.height,
        minHeight: compact ? 32 : SMALL_NODE_STYLES.height,
        padding: SMALL_NODE_STYLES.padding,
        borderRadius: theme.shape.borderRadius,
        border: '1px solid',
        borderColor: active
            ? getRouteGraphActiveColor(theme)
            : alpha(getRouteGraphActiveColor(theme), 0.4),
        backgroundColor: theme.palette.background.paper,
        boxShadow: 'none',
        transition: 'border-color 0.16s ease, background-color 0.16s ease, opacity 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
        cursor: active ? 'pointer' : 'default',
        opacity: active ? 1 : 0.5,
        outline: 'none',
        '&:hover': active ? {
            borderColor: getRouteGraphActiveColor(theme),
            boxShadow: [
                `0 0 0 4px ${alpha(getRouteGraphActiveColor(theme), 0.18)}`,
                '0 14px 34px rgba(31, 41, 55, 0.14)',
                '0 3px 10px rgba(31, 41, 55, 0.08)',
            ].join(', '),
            transform: 'translateY(-2px)',
        } : {},
    })
);

const ToggleButtonStyled = styled(ToggleButton)<{ compact?: boolean }>(
    ({ theme, compact }) => ({
        ...NODE_LAYER_STYLES.toggleButton,
        flex: 1,
        width: '100%',
        height: compact ? 20 : 22,
        fontSize: compact ? '0.6rem' : '0.65rem',
        padding: compact ? '0 4px' : '0 6px',
        borderColor: alpha(getRouteGraphActiveColor(theme), 0.7),
        color: theme.palette.text.secondary,
        borderRadius: theme.shape.borderRadius,
        '&.Mui-selected': {
            backgroundColor: getRouteGraphControlFill(theme),
            color: theme.palette.common.white,
            borderColor: getRouteGraphControlFill(theme),
            '& .MuiSvgIcon-root': {
                color: theme.palette.common.white,
            },
            '&:hover': {
                backgroundColor: getRouteGraphControlFillHover(theme),
            },
        },
        '&:hover': {
            backgroundColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.16 : 0.08),
        },
    })
);

// EntryNode Props
export interface EntryNodeProps {
    active: boolean;
    smartEnabled: boolean;
    onSwitch?: () => void;
    switchDisabled?: boolean;
    compact?: boolean;  // Horizontal layout
    orientation?: 'horizontal' | 'vertical';  // Alias for compact
}

/**
 * EntryNode - A specialized node containing only Direct/Smart mode toggle buttons.
 *
 * This component separates the routing mode selection from the model name display,
 * allowing for a cleaner layout where the model name and mode toggle can be
 * positioned independently.
 *
 * Layout options:
 * - vertical (default): Buttons stacked vertically
 * - horizontal/compact: Buttons arranged horizontally side by side
 */
export const EntryNode: React.FC<EntryNodeProps> = ({
    active,
    smartEnabled,
    onSwitch,
    switchDisabled = false,
    compact = false,
    orientation,
}) => {
    // Support both compact and orientation props
    const isCompact = compact || orientation === 'horizontal';

    return (
        <StyledEntryNode active={active} compact={isCompact}>
            {isCompact ? (
                // Horizontal layout - buttons side by side
                <>
                    <ToggleButtonStyled
                        value="direct"
                        selected={!smartEnabled}
                        disabled={!active || switchDisabled}
                        onClick={onSwitch}
                        aria-label="Direct routing mode"
                        aria-pressed={!smartEnabled}
                    >
                        <DirectIcon sx={{ fontSize: 9, transform: 'rotate(90deg)' }} />
                        Direct
                    </ToggleButtonStyled>
                    <ToggleButtonStyled
                        value="smart"
                        selected={smartEnabled}
                        disabled={!active || switchDisabled}
                        onClick={onSwitch}
                        aria-label="Smart routing mode"
                        aria-pressed={smartEnabled}
                    >
                        <AutoAwesomeIcon sx={{ fontSize: 9 }} />
                        Smart
                    </ToggleButtonStyled>
                </>
            ) : (
                // Vertical layout - buttons stacked
                <>
                    <ToggleButtonStyled
                        value="direct"
                        selected={!smartEnabled}
                        disabled={!active || switchDisabled}
                        onClick={onSwitch}
                        aria-label="Direct routing mode"
                        aria-pressed={!smartEnabled}
                    >
                        <DirectIcon sx={{ fontSize: 10, transform: 'rotate(90deg)' }} />
                        Direct
                    </ToggleButtonStyled>
                    <ToggleButtonStyled
                        value="smart"
                        selected={smartEnabled}
                        disabled={!active || switchDisabled}
                        onClick={onSwitch}
                        aria-label="Smart routing mode"
                        aria-pressed={smartEnabled}
                    >
                        <AutoAwesomeIcon sx={{ fontSize: 10 }} />
                        Smart
                    </ToggleButtonStyled>
                </>
            )}
        </StyledEntryNode>
    );
};

export default EntryNode;
