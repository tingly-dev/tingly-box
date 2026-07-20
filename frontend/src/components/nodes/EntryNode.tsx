import { ToggleButton, Tooltip, Box } from '@mui/material';
import { alpha, styled } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
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
import { NodeTooltip } from './NodeTooltip';

// Styled EntryNode matching ActionAddNode style
const StyledEntryNode = styled('div')<{ active: boolean; compact?: boolean }>(
    ({ active, compact, theme }) => ({
        display: 'flex',
        flexDirection: compact ? 'row' : 'column',
        justifyContent: 'center',
        alignItems: 'center',
        gap: 4,
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
        position: 'relative',
        '&:hover': active ? {
            borderColor: getRouteGraphActiveColor(theme),
            boxShadow: [
                `0 0 0 4px ${alpha(getRouteGraphActiveColor(theme), 0.18)}`,
                '0 14px 34px rgba(31, 41, 55, 0.14)',
                '0 3px 10px rgba(31, 41, 55, 0.08)',
            ].join(','),
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
    onShowDirectGuide?: () => void;
    onShowSmartGuide?: () => void;
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
    onShowDirectGuide,
    onShowSmartGuide,
}) => {
    const { t } = useTranslation();
    // Support both compact and orientation props
    const isCompact = compact || orientation === 'horizontal';

    const directTitle = t('rule.routing.directTooltipTitle', { defaultValue: 'Direct Routing' });
    const directBody = t('rule.routing.directTooltipBody', { defaultValue: 'Load balance across all services in tier order. Simple and predictable.' });
    const smartTitle = t('rule.routing.smartTooltipTitle', { defaultValue: 'Smart Routing' });
    const smartBody = t('rule.routing.smartTooltipBody', { defaultValue: 'Route based on custom conditions like model name, token count, or user groups.' });

    // Enhanced tooltip with both Direct and Smart info
    const tooltipContent = (
        <Box sx={{ whiteSpace: 'pre-line', maxWidth: 280 }}>
            <strong>{directTitle}</strong>
            {`\n${directBody}\n\n`}
            <strong>{smartTitle}</strong>
            {`\n${smartBody}\n\n`}
            <Box component="span" sx={{ opacity: 0.7 }}>
                {t('rule.routing.tooltipHint', { defaultValue: 'Click a button to switch modes' })}
            </Box>
            {(onShowDirectGuide || onShowSmartGuide) && (
                <Box sx={{ mt: 1 }}>
                    {onShowDirectGuide && (
                        <Box
                            component="span"
                            onClick={(e) => { e.stopPropagation(); onShowDirectGuide(); }}
                            sx={{
                                display: 'block',
                                color: 'primary.main',
                                cursor: 'pointer',
                                fontWeight: 500,
                                '&:hover': { textDecoration: 'underline' }
                            }}
                        >
                            {t('rule.routing.viewDirectGuide', { defaultValue: 'View direct routing guide →' })}
                        </Box>
                    )}
                    {onShowSmartGuide && (
                        <Box
                            component="span"
                            onClick={(e) => { e.stopPropagation(); onShowSmartGuide(); }}
                            sx={{
                                display: 'block',
                                color: 'primary.main',
                                cursor: 'pointer',
                                fontWeight: 500,
                                '&:hover': { textDecoration: 'underline' }
                            }}
                        >
                            {t('rule.routing.viewSmartGuide', { defaultValue: 'View smart routing guide →' })}
                        </Box>
                    )}
                </Box>
            )}
        </Box>
    );

    return (
        <NodeTooltip title={tooltipContent} placement="top">
            <StyledEntryNode active={active} compact={isCompact}>
                {isCompact ? (
                    // Horizontal layout - buttons side by side
                    (<>
                        <ToggleButtonStyled
                            value="direct"
                            selected={!smartEnabled}
                            disabled={!active || switchDisabled}
                            onClick={smartEnabled ? onSwitch : undefined}
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
                            onClick={smartEnabled ? undefined : onSwitch}
                            aria-label="Smart routing mode"
                            aria-pressed={smartEnabled}
                        >
                            <AutoAwesomeIcon sx={{ fontSize: 9 }} />
                            Smart
                        </ToggleButtonStyled>
                    </>)
                ) : (
                    // Vertical layout - buttons stacked
                    (<>
                        <ToggleButtonStyled
                            value="direct"
                            selected={!smartEnabled}
                            disabled={!active || switchDisabled}
                            onClick={smartEnabled ? onSwitch : undefined}
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
                            onClick={smartEnabled ? undefined : onSwitch}
                            aria-label="Smart routing mode"
                            aria-pressed={smartEnabled}
                        >
                            <AutoAwesomeIcon sx={{ fontSize: 10 }} />
                            Smart
                        </ToggleButtonStyled>
                    </>)
                )}
            </StyledEntryNode>
        </NodeTooltip>
    );
};

export default EntryNode;
