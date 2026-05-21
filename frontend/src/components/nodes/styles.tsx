import { Box } from '@mui/material';
import { alpha, styled, type Theme } from '@mui/material/styles';

// Node dimensions constants
export const MODEL_NODE_STYLES = {
    width: 220,
    height: 72,
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
    const emphasisColor = isDark ? theme.palette.primary.light : theme.palette.primary.dark;
    const tintAlpha = isDark ? 0.12 : 0.045;

    return {
        borderColor: emphasisColor,
        backgroundColor: alpha(theme.palette.primary.main, tintAlpha),
        color: emphasisColor,
        '& .MuiTypography-root': {
            color: emphasisColor,
        },
        '& .MuiSvgIcon-root': {
            color: emphasisColor,
        },
        boxShadow: isDark
            ? [
                '0 14px 30px rgba(0, 0, 0, 0.38)',
                `0 0 0 1px ${alpha(emphasisColor, 0.22)}`,
            ].join(', ')
            : [
                '0 10px 24px rgba(15, 23, 42, 0.12)',
                '0 2px 8px rgba(15, 23, 42, 0.08)',
                `0 0 0 1px ${alpha(emphasisColor, 0.16)}`,
            ].join(', '),
        transform: 'translateY(-1px)',
    };
};

export const graphNodeBaseHoverStyles = {
    outline: 'none',
    boxShadow: 'none',
    transform: 'translateY(0)',
} as const;

// Provider node container
export const ProviderNodeContainer = styled(Box)(({ theme }: { theme: Theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    padding: providerNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    width: providerNode.width,
    height: providerNode.height,
    boxShadow: 'none',
    transition: 'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    position: 'relative',
    ...graphNodeBaseHoverStyles,
    '&:hover': graphNodeHoverStyles(theme),
}));

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
    borderColor: 'divider',
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

// Base smart node styles
const baseSmartNodeStyles = ({ active, theme }: { active: boolean; theme: Theme }) => ({
    display: 'flex',
    flexDirection: 'column' as const,
    alignItems: 'center',
    justifyContent: 'center',
    padding: smartNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: active ? 'text.secondary' : 'divider',
    backgroundColor: active ? 'action.hover' : 'background.paper',
    textAlign: 'center' as const,
    width: smartNode.width,
    height: smartNode.height,
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
    divider: { width: '78%', my: 0.125 } as const,
    bottomLayer: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '100%',
        minHeight: 18,
    } as const,
    typography: { fontWeight: 600, fontSize: '0.8rem', lineHeight: 1.15 } as const,
    toggleButton: {
        height: 20,
        padding: '0 6px',
        fontSize: '0.6rem',
        fontWeight: 600,
        textTransform: 'none' as const,
        border: '1px solid',
        borderRadius: 1,
    } as const,
};
