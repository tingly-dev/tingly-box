import { Box } from '@mui/material';
import { styled } from '@mui/material/styles';

// Model Node dimensions
export const MODEL_NODE_STYLES = {
    width: 220,
    height: 90,
    heightCompact: 60,
    widthCompact: 220,
    padding: 8,
} as const;

// Provider Node dimensions
export const PROVIDER_NODE_STYLES = {
    width: 220,
    height: 90,
    heightCompact: 60,
    padding: 8,
    widthCompact: 320,
    // Internal dimensions
    badgeHeight: 5,
    fieldHeight: 5,
    fieldPadding: 2,
    elementMargin: 0.5,
} as const;

// Smart Node dimensions (used by SmartOpNode and SmartFallbackNode)
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

// Container for graph nodes
export const NodeContainer = styled(Box)(() => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 8,
}));

// Connection line between nodes
export const ConnectionLine = styled(Box)(({ }) => ({
    display: 'flex',
    alignItems: 'center',
    color: 'text.secondary',
    fontSize: '1.5rem',
    '& svg': {
        fontSize: '2rem',
    }
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
        borderColor: 'primary.main',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    }
}));

// Styled model node with unified fixed size
export const StyledModelNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'compact',
})<{ compact?: boolean }>(({ compact, theme }) => ({
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
        borderColor: 'primary.main',
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    }
}));

// Action button container (shared by SmartOpNode and SmartFallbackNode)
export const ActionButtonsBox = styled(Box)(({ theme }) => ({
    position: 'absolute',
    top: 4,
    right: 4,
    display: 'flex',
    gap: 2,
    opacity: 0,
    transition: 'opacity 0.2s',
}));

// Smart node wrapper (shared by SmartOpNode and SmartFallbackNode)
export const StyledSmartNodeWrapper = styled(Box)(({ theme }) => ({
    position: 'relative',
    '&:hover .action-buttons': {
        opacity: 1,
    }
}));

// Base smart node styles - used to create themed variants
const baseSmartNodeStyles = ({ active, theme, color }: {
    active: boolean;
    theme: any;
    color: 'primary' | 'warning' | 'secondary' | 'error' | 'info' | 'success';
}) => ({
    display: 'flex',
    flexDirection: 'column' as const,
    alignItems: 'center',
    justifyContent: 'center',
    padding: smartNode.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: active ? `${color}.main` : 'divider',
    backgroundColor: active ? `${color}.50` : 'background.paper',
    textAlign: 'center',
    width: smartNode.width,
    height: smartNode.height,
    boxShadow: theme.shadows[2],
    transition: 'all 0.2s ease-in-out',
    position: 'relative' as const,
    opacity: active ? 1 : 0.6,
    '&:hover': {
        borderColor: `${color}.main`,
        backgroundColor: `${color}.100`,
        boxShadow: theme.shadows[4],
        transform: 'translateY(-2px)',
    }
});

// Smart node with primary color theme (used by SmartOpNode)
export const StyledSmartNodePrimary = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => baseSmartNodeStyles({ active, theme, color: 'primary' }));

// Smart node with warning color theme (used by SmartFallbackNode)
export const StyledSmartNodeWarning = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => baseSmartNodeStyles({ active, theme, color: 'warning' }));
