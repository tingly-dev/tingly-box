import { Box } from '@mui/material';
import { alpha, keyframes, styled, type Theme } from '@mui/material/styles';

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

// Diagonal-hatch overlay for anything "deliberately not running" (off bot
// card, unmounted purpose, blocklisted chat leaf) — distinct from dimmed =
// "upstream is off". Host must be position: relative; pointer-transparent so
// everything underneath stays interactive.
/** @deprecated Use getInactiveHatchSx(theme) — this static variant is invisible in dark mode. */
export const inactiveHatchSx = {
    '&::before': {
        content: '""',
        position: 'absolute' as const,
        inset: 0,
        borderRadius: 'inherit',
        zIndex: 2,
        pointerEvents: 'none' as const,
        backgroundImage:
            'repeating-linear-gradient(45deg, transparent, transparent 10px, rgba(0,0,0,0.035) 10px, rgba(0,0,0,0.035) 20px)',
    },
} as const;

// Theme-aware hatch: dark paper needs light stripes (the rgba-black variant
// above disappears against it).
export const getInactiveHatchSx = (theme: Theme) => ({
    '&::before': {
        content: '""',
        position: 'absolute' as const,
        inset: 0,
        borderRadius: 'inherit',
        zIndex: 2,
        pointerEvents: 'none' as const,
        backgroundImage:
            theme.palette.mode === 'dark'
                ? 'repeating-linear-gradient(45deg, transparent, transparent 10px, rgba(255,255,255,0.055) 10px, rgba(255,255,255,0.055) 20px)'
                : 'repeating-linear-gradient(45deg, transparent, transparent 10px, rgba(0,0,0,0.035) 10px, rgba(0,0,0,0.035) 20px)',
    },
} as const);

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

export const { providerNode, smartNode } = {
    providerNode: PROVIDER_NODE_STYLES,
    smartNode: SMART_NODE_STYLES,
};

// ActionAddNode dimensions
export const SMALL_NODE_STYLES = {
    width: 100,
    height: 72,
    padding: 5,
} as const;

// Remote/notify graph card nodes (ImBot, Agent, BotModel, CCProfile, Chat,
// ApiEntry, CWD, RoutingMode) — same footprint as the route-graph model
// nodes so all graphs read at one scale.
export const BOT_NODE_STYLES = {
    width: 220,
    height: 76,
    padding: 5,
} as const;

// AgentConfigNode — a deliberately smaller supplementary node.
export const AGENT_CONFIG_NODE_STYLES = {
    width: 180,
    height: 60,
    padding: 8,
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

// "Needs attention" border for bot-graph nodes (unconfigured model, missing
// CC profile) — the warning signal in the new visual language: amber border,
// no tint background.
export const getBotGraphWarnBorderColor = (theme: Theme) =>
    alpha(theme.palette.warning.main, theme.palette.mode === 'dark' ? 0.72 : 0.70);

// Shared base for the remote/notify graph card family. Replaces the per-node
// styled(Box) copies (shadows[2] + semantic tints) with the route-graph
// language: neutral alpha border, paper background, no static shadow, hover
// emphasis ring only when clickable, 0.6 opacity when inactive.
export const StyledBotGraphNode = styled(Box, {
    shouldForwardProp: (prop) =>
        prop !== 'active' && prop !== 'clickable' && prop !== 'warn',
})<{ active?: boolean; clickable?: boolean; warn?: boolean }>(
    ({ active = true, clickable = false, warn = false, theme }) => ({
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        padding: BOT_NODE_STYLES.padding,
        borderRadius: theme.shape.borderRadius,
        border: '1px solid',
        borderColor: warn
            ? getBotGraphWarnBorderColor(theme)
            : getRouteGraphBorderColor(theme),
        backgroundColor: theme.palette.background.paper,
        textAlign: 'center',
        width: BOT_NODE_STYLES.width,
        height: BOT_NODE_STYLES.height,
        transition:
            'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
        position: 'relative',
        opacity: active ? 1 : 0.6,
        cursor: clickable ? 'pointer' : 'default',
        ...graphNodeBaseHoverStyles,
        ...(clickable && { '&:hover': graphNodeHoverStyles(theme) }),
    }),
);

// Horizontal graph row with a thin, mode-aware scrollbar — shared by
// RemoteControlGraph and the notify graph so both scroll surfaces match.
export const graphRowStyles = (theme: Theme) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'flex-start',
    gap: theme.spacing(1.5),
    flexWrap: 'nowrap' as const,
    overflowX: 'auto' as const,
    overflowY: 'visible' as const,
    paddingBottom: theme.spacing(0.5),
    scrollbarWidth: 'thin' as const,
    scrollbarColor:
        (theme.palette.mode === 'dark' ? '#555' : '#ccc') + ' transparent',
    '&::-webkit-scrollbar': { height: 6 },
    '&::-webkit-scrollbar-track': { background: 'transparent' },
    '&::-webkit-scrollbar-thumb': {
        backgroundColor: theme.palette.mode === 'dark' ? '#555' : '#ccc',
        borderRadius: 3,
        '&:hover': {
            backgroundColor: theme.palette.mode === 'dark' ? '#777' : '#999',
        },
    },
});

// Spotlight: a primary-colored ring that briefly pulses to draw attention to a
// node when guidance (Quick Start → "Select a Model") points the user at it.
// Shared by ActionAddNode (add a model) and ServiceNode (edit an existing one).
export const spotlightPulse = keyframes`
    0%   { box-shadow: 0 0 0 0 var(--node-spotlight-ring); }
    70%  { box-shadow: 0 0 0 9px transparent; }
    100% { box-shadow: 0 0 0 0 transparent; }
`;

export const nodeSpotlightSx = (theme: Theme) => ({
    borderColor: theme.palette.primary.main,
    '--node-spotlight-ring': alpha(theme.palette.primary.main, 0.5),
    animation: `${spotlightPulse} 1.4s ease-out 3`,
});

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
    transition: 'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    position: 'relative',
    ...graphNodeBaseHoverStyles,
    '&:hover': graphNodeHoverStyles(theme),
}));

/** @deprecated Use ServiceNodeContainer */
export const ProviderNodeContainer = ServiceNodeContainer;

// Action button container — appears on hover with a solid backing so buttons
// are always readable regardless of node opacity or background content.
export const ActionButtonsBox = styled(Box)(({ theme }: { theme: Theme }) => ({
    position: 'absolute',
    top: 0,
    right: 0,
    display: 'flex',
    gap: 2,
    opacity: 0,
    transition: 'opacity 0.2s, transform 0.2s',
    backgroundColor: theme.palette.background.paper,
    borderRadius: theme.shape.borderRadius,
    padding: '4px',
    boxShadow: theme.shadows[2],
    border: '1px solid',
    borderColor: alpha(getRouteGraphBorderColor(theme), 0.6),
    zIndex: 10,
    '&:hover': {
        transform: 'translateY(-2px)',
    },
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
        fontSize: '0.75rem',
        fontWeight: 600,
        textTransform: 'none' as const,
        border: '1px solid',
        borderRadius: 1.25,
        lineHeight: 1,
    } as const,
};
