import { Box } from '@mui/material';
import { styled } from '@mui/material/styles';

// Node dimensions constants
export const MODEL_NODE_STYLES = {
    width: 220,
    height: 90,
    heightCompact: 60,
    widthCompact: 220,
    padding: 8,
} as const;

export const PROVIDER_NODE_STYLES = {
    width: 220,
    height: 90,
    heightCompact: 60,
    padding: 8,
    widthCompact: 320,
    badgeHeight: 5,
    fieldHeight: 5,
    fieldPadding: 2,
    elementMargin: 0.5,
} as const;

export const SMART_NODE_STYLES = {
    width: 220,
    height: 90,
    padding: 8,
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

// Provider node container
export const ProviderNodeContainer = styled(Box)(({ theme }) => ({
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
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    '&:hover': {
        borderColor: 'text.secondary',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    },
}));

// Styled model node with unified fixed size
export const StyledModelNode = styled(Box, { shouldForwardProp: (prop) => prop !== 'compact' })<{
    compact?: boolean;
}>(({ compact, theme }) => ({
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
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative',
    cursor: 'pointer',
    '&:hover': {
        borderColor: 'text.secondary',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    },
}));

// Action button container
export const ActionButtonsBox = styled(Box)(({ theme }) => ({
    position: 'absolute',
    top: 4,
    right: 4,
    display: 'flex',
    gap: 2,
    opacity: 0,
    transition: 'opacity 0.2s',
}));

// Smart node wrapper
export const StyledSmartNodeWrapper = styled(Box)(({ theme }) => ({
    position: 'relative',
    '&:hover .action-buttons': { opacity: 1 },
}));

// Base smart node styles
const baseSmartNodeStyles = ({ active, theme }: { active: boolean; theme: any }) => ({
    display: 'flex',
    flexDirection: 'column' as const,
    alignItems: 'center',
    justifyContent: 'center',
    padding: smartNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: active ? 'text.secondary' : 'divider',
    backgroundColor: active ? 'action.hover' : 'background.paper',
    textAlign: 'center',
    width: smartNode.width,
    height: smartNode.height,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative' as const,
    opacity: active ? 1 : 0.6,
    '&:hover': {
        borderColor: 'text.secondary',
        backgroundColor: 'action.hover',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    },
});

export const StyledSmartNodePrimary = styled(Box, { shouldForwardProp: (prop) => prop !== 'active' })<{
    active: boolean;
}>(({ active, theme }) => baseSmartNodeStyles({ active, theme }));

export const StyledSmartNodeWarning = styled(Box, { shouldForwardProp: (prop) => prop !== 'active' })<{
    active: boolean;
}>(({ active, theme }) => baseSmartNodeStyles({ active, theme }));

// Shared node layer styles for two-layer layout
export const NODE_LAYER_STYLES = {
    topLayer: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '100%',
    } as const,
    divider: { width: '80%', my: 0.5 } as const,
    bottomLayer: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: '100%',
        minHeight: 24,
    } as const,
    typography: { fontWeight: 600, fontSize: '0.9rem' } as const,
    toggleButton: {
        height: 24,
        padding: '0 8px',
        fontSize: '0.65rem',
        fontWeight: 600,
        textTransform: 'none' as const,
        border: '1px solid',
        borderRadius: 1,
    } as const,
};
