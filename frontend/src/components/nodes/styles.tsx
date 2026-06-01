import { Box } from '@mui/material';
import { alpha, styled, type Theme } from '@mui/material/styles';

export const routeGraphActive = '#4F6F9F';
export const routeGraphActiveBg = '#F7F9FC';

export const getRouteGraphActiveColor = (theme: Theme) =>
    theme.palette.mode === 'dark' ? '#D4E3FF' : routeGraphActive;

export const getRouteGraphControlFill = (theme: Theme) =>
    theme.palette.mode === 'dark' ? '#4F6F9F' : routeGraphActive;

export const getRouteGraphControlFillHover = (theme: Theme) =>
    theme.palette.mode === 'dark' ? '#5F82BA' : routeGraphActive;

export const getRouteGraphActiveBg = (theme: Theme) =>
    theme.palette.mode === 'dark' ? alpha(routeGraphActive, 0.18) : routeGraphActiveBg;

export const getRouteGraphBorderColor = (theme: Theme) =>
    alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.48 : 0.50);

// Node dimensions constants
export const MODEL_NODE_STYLES = {
    width: 220,
    height: 76,
    heightCompact: 48,
    widthCompact: 220,
    padding: 5,
} as const;

export const PROVIDER_NODE_STYLES = {
    width: 220,
    height: 72,
    heightCompact: 48,
    padding: 5,
    widthCompact: 320,
    badgeHeight: 5,
    fieldHeight: 5,
    fieldPadding: 2,
    elementMargin: 0.5,
} as const;

export const SMART_NODE_STYLES = {
    width: 220,
    height: 72,
    padding: 5,
} as const;

export const { modelNode, providerNode, smartNode } = {
    modelNode: MODEL_NODE_STYLES,
    providerNode: PROVIDER_NODE_STYLES,
    smartNode: SMART_NODE_STYLES,
};

// ActionAddNode dimensions
export const SMALL_NODE_STYLES = {
    width: 100,
    height: 72,
    padding: 5,
} as const;

// Common styled components
export const NodeContainer = styled(Box)(() => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 8,
}));

export const ConnectionLine = styled(Box)(() => ({
    display: 'flex',
    alignItems: 'center',
    color: 'text.secondary',
    fontSize: '1.5rem',
    '& svg': { fontSize: '2rem' },
}));

export const graphNodeHoverStyles = (theme: Theme) => {
    const isDark = theme.palette.mode === 'dark';
    const emphasisColor = getRouteGraphActiveColor(theme);

    return {
        borderColor: emphasisColor,
        color: emphasisColor,
        '& .MuiTypography-root': {
            color: emphasisColor,
        },
        '& .MuiSvgIcon-root': {
            color: emphasisColor,
        },
        boxShadow: isDark
            ? [
                `0 0 0 1px ${alpha(emphasisColor, 0.92)}`,
                `0 0 0 5px ${alpha(emphasisColor, 0.34)}`,
                '0 18px 38px rgba(0, 0, 0, 0.50)',
            ].join(', ')
            : [
                `0 0 0 4px ${alpha(routeGraphActive, 0.18)}`,
                '0 14px 34px rgba(31, 41, 55, 0.14)',
                '0 3px 10px rgba(31, 41, 55, 0.08)',
            ].join(', '),
        transform: 'translateY(-2px)',
    };
};

export const graphNodeBaseHoverStyles = {
    outline: 'none',
    boxShadow: 'none',
    transform: 'translateY(0)',
} as const;

// Service node container (formerly ProviderNodeContainer)
export const ServiceNodeContainer = styled(Box)(({ theme }: { theme: Theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    padding: providerNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: 'background.paper',
    width: providerNode.width,
    height: providerNode.height,
    boxShadow: 'none',
    transition: 'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    position: 'relative',
    ...graphNodeBaseHoverStyles,
    '&:hover': graphNodeHoverStyles(theme),
}));

/** @deprecated Use ServiceNodeContainer */
export const ProviderNodeContainer = ServiceNodeContainer;

// Styled model node with unified fixed size
export const StyledModelNode = styled(Box, { shouldForwardProp: (prop) => prop !== 'compact' })<{
    compact?: boolean;
}>(({ compact, theme }: { compact?: boolean; theme: Theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: modelNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: 'background.paper',
    textAlign: 'center',
    width: compact ? modelNode.widthCompact : modelNode.width,
    height: compact ? modelNode.heightCompact : modelNode.height,
    boxShadow: 'none',
    transition: 'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    position: 'relative',
    cursor: 'pointer',
    ...graphNodeBaseHoverStyles,
    '&:hover': graphNodeHoverStyles(theme),
}));

// Action button container
export const ActionButtonsBox = styled(Box)(({ theme }: { theme: Theme }) => ({
    position: 'absolute',
    top: 4,
    right: 4,
    display: 'flex',
    gap: 2,
    opacity: 0,
    transition: 'opacity 0.2s',
}));

// Smart node wrapper
export const StyledSmartNodeWrapper = styled(Box)(({ theme }: { theme: Theme }) => ({
    position: 'relative',
    '&:hover .action-buttons': { opacity: 1 },
}));

// Base smart node styles — dashed border + flexible height to fit op-tag rows.
const baseSmartNodeStyles = ({ active, theme }: { active: boolean; theme: Theme }) => ({
    display: 'flex',
    flexDirection: 'column' as const,
    alignItems: 'stretch',
    justifyContent: 'flex-start',
    padding: smartNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: 'background.paper',
    width: smartNode.width,
    minHeight: smartNode.height,
    boxShadow: 'none',
    transition: 'border-color 0.16s ease, background-color 0.16s ease, opacity 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    position: 'relative' as const,
    opacity: active ? 1 : 0.6,
    ...graphNodeBaseHoverStyles,
    '&:hover': graphNodeHoverStyles(theme),
});

export const StyledSmartNodePrimary = styled(Box, { shouldForwardProp: (prop) => prop !== 'active' })<{
    active?: boolean;
}>(({ active = false, theme }) => baseSmartNodeStyles({ active, theme }));

export const StyledSmartNodeWarning = styled(Box, { shouldForwardProp: (prop) => prop !== 'active' })<{
    active?: boolean;
}>(({ active = false, theme }) => baseSmartNodeStyles({ active, theme }));

// Shared node layer styles for two-layer layout
export const NODE_LAYER_STYLES = {
    topLayer: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '100%',
    } as const,
    divider: { width: '84%', my: 0.25 } as const,
    bottomLayer: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '100%',
        minHeight: 26,
        px: 0.5,
        gap: 0.5,
    } as const,
    typography: { fontWeight: 600, fontSize: '0.8rem', lineHeight: 1.15 } as const,
    toggleButton: {
        height: 24,
        minWidth: 0,
        padding: '0 8px',
        gap: 0.5,
        fontSize: '0.68rem',
        fontWeight: 600,
        textTransform: 'none' as const,
        border: '1px solid',
        borderRadius: 1.25,
        lineHeight: 1,
    } as const,
};
