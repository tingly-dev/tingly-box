import { Add as AddIcon, Close as CloseIcon, Extension as ExtensionIcon } from '@/components/icons';
import { Box, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { alpha, styled } from '@mui/material/styles';
import React from 'react';
import {
    getRouteGraphActiveColor,
    getRouteGraphBorderColor,
    graphNodeBaseHoverStyles,
    graphNodeHoverStyles,
    MODEL_NODE_STYLES,
} from '@/components/nodes/styles';
import type { FlagSpec, RuleFlags } from '@/components/RoutingGraphTypes';

// Matches the route graph node footprint so this feels like a pinned tool node,
// not a smaller floating card. Content overflow scrolls inside the body.
const CARD_STYLES = {
    width: MODEL_NODE_STYLES.width,
    minHeight: MODEL_NODE_STYLES.height,
    padding: 8,
} as const;

const CARD_HEADER_HEIGHT = 18;

const StyledExtensionsCard = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active',
})<{ active: boolean }>(({ active, theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    padding: CARD_STYLES.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px dashed',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: theme.palette.background.paper,
    width: CARD_STYLES.width,
    minHeight: CARD_STYLES.minHeight,
    maxHeight: '100%',
    boxShadow: 'none',
    opacity: active ? 1 : 0.6,
    cursor: 'pointer',
    transition: 'border-color 0.16s ease, background-color 0.16s ease, opacity 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    overflow: 'hidden',
    ...graphNodeBaseHoverStyles,
    '&:hover': {
        ...graphNodeHoverStyles(theme),
    },
}));

export interface RuleExtensionsCardProps {
    flags?: RuleFlags;
    registry?: FlagSpec[];
    active: boolean;
    onOpenCatalog: () => void;
    onToggleFlag?: (key: string) => void;
}

const flagBoolValue = (flags: RuleFlags | undefined, key: string): boolean => {
    if (!flags) return false;
    switch (key) {
        case 'cursor_compat':
            return !!flags.cursorCompat;
        case 'cursor_compat_auto':
            return !!flags.cursorCompatAuto;
        case 'skip_usage':
            return !!flags.skipUsage;
        case 'use_max_completion_tokens':
            return !!flags.useMaxCompletionTokens;
        case 'use_max_tokens':
            return !!flags.useMaxTokens;
        default:
            return false;
    }
};

const flagIntValue = (flags: RuleFlags | undefined, key: string): number => {
    if (!flags) return 0;
    switch (key) {
        case 'session_affinity':
            return flags.sessionAffinity ?? 0;
        default:
            return 0;
    }
};

const formatSeconds = (s: number): string => {
    if (s <= 0) return '0s';
    if (s % 3600 === 0) return `${s / 3600}h`;
    if (s % 60 === 0) return `${s / 60}m`;
    return `${s}s`;
};

const flagStringValue = (flags: RuleFlags | undefined, key: string): string => {
    if (!flags) return '';
    switch (key) {
        case 'custom_user_agent':
            return flags.customUserAgent || '';
        case 'openai_endpoint_override':
            return flags.openaiEndpointOverride || '';
        case 'block_tools':
            return flags.blockTools || '';
        case 'thinking_effort':
            return flags.thinkingEffort || '';
        default:
            return '';
    }
};

// flagServiceRefDisplay returns the model name of a service_ref flag (the
// concise label for the extension chip), or '' when unset.
const flagServiceRefDisplay = (flags: RuleFlags | undefined, key: string): string => {
    if (!flags) return '';
    switch (key) {
        case 'vision_proxy_service':
            return flags.visionProxyService?.model || '';
        default:
            return '';
    }
};

/**
 * RuleExtensionsCard renders a compact card displaying the rule's enabled
 * extension flags. The "+ Add" action opens the catalog dialog where users
 * pick which flags to enable and supply any parameters they require.
 */
export const RuleExtensionsCard: React.FC<RuleExtensionsCardProps> = ({
    flags,
    registry,
    active,
    onOpenCatalog,
    onToggleFlag,
}) => {
    const enabled = (registry || []).filter((spec) => {
        if (spec.type === 'bool') return flagBoolValue(flags, spec.key);
        if (spec.type === 'int') return flagIntValue(flags, spec.key) > 0;
        if (spec.type === 'enum') {
            const v = flagStringValue(flags, spec.key);
            const inactive = spec.options?.[0]?.value ?? '';
            return v !== '' && v !== inactive;
        }
        if (spec.type === 'service_ref') return flagServiceRefDisplay(flags, spec.key) !== '';
        return flagStringValue(flags, spec.key) !== '';
    });

    return (
        <StyledExtensionsCard active={active} onClick={onOpenCatalog}>
            {/* Fixed-height header so the body has a stable scroll region */}
            <Stack
                direction="row"
                alignItems="center"
                spacing={0.75}
                sx={{
                    flexShrink: 0,
                    height: CARD_HEADER_HEIGHT,
                    mb: 0.75,
                }}
            >
                <ExtensionIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                <Typography variant="caption" sx={{ fontWeight: 700, fontSize: '0.72rem', color: 'text.secondary', flexGrow: 1, lineHeight: 1 }}>
                    Extensions{enabled.length > 0 ? ` (${enabled.length})` : ''}
                </Typography>
                {/* Visual affordance only — the whole card is clickable. */}
                <Tooltip title="Configure rule extensions">
                    <AddIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                </Tooltip>
            </Stack>

            {enabled.length === 0 ? (
                <Box
                    sx={{
                        flexGrow: 1,
                        minHeight: 0,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'text.disabled',
                        fontSize: '0.72rem',
                        lineHeight: 1.25,
                        textAlign: 'center',
                        px: 1,
                    }}
                >
                    None enabled. Click to configure.
                </Box>
            ) : (
                <Box
                    sx={{
                        flexGrow: 1,
                        minHeight: 0,
                        pr: 0.25,
                        overflowY: 'auto',
                        // Hide scrollbar visually but keep it functional.
                        scrollbarWidth: 'thin',
                        scrollbarGutter: 'stable',
                        '&::-webkit-scrollbar': { width: 5 },
                        '&::-webkit-scrollbar-track': { backgroundColor: 'transparent' },
                        '&::-webkit-scrollbar-thumb': {
                            backgroundColor: (theme) => alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.34 : 0.24),
                            borderRadius: 3,
                        },
                        '&::-webkit-scrollbar-thumb:hover': {
                            backgroundColor: (theme) => alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.5 : 0.36),
                        },
                    }}
                    onClick={(e) => e.stopPropagation()}
                    onDoubleClick={(e) => e.stopPropagation()}
                >
                    <Stack gap={0.4}>
                        {enabled.map((spec) => {
                            const isString = spec.type === 'string';
                            const isEnum = spec.type === 'enum';
                            const isServiceRef = spec.type === 'service_ref';
                            const stringVal = isString || isEnum ? flagStringValue(flags, spec.key) : '';
                            let displayVal = stringVal;
                            if (isEnum) {
                                const opt = (spec.options || []).find((o) => o.value === stringVal);
                                if (opt) displayVal = opt.label;
                            }
                            if (isServiceRef) displayVal = flagServiceRefDisplay(flags, spec.key);
                            const tooltipTitle = ((isString || isEnum) && stringVal) || (isServiceRef && displayVal)
                                ? `${spec.description}\nValue: ${displayVal}`
                                : spec.description;
                            return (
                                <Tooltip key={spec.key} title={spec.type === 'int' ? `${spec.description}\nValue: ${formatSeconds(flagIntValue(flags, spec.key))}` : tooltipTitle} placement="left">
                                    <Box
                                        sx={(theme) => ({
                                            width: '100%',
                                            px: 0.75,
                                            py: 0.35,
                                            borderRadius: 0.75,
                                            border: '1px solid',
                                            borderColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.28 : 0.18),
                                            backgroundColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.07 : 0.03),
                                            display: 'flex',
                                            alignItems: 'center',
                                            gap: 0.5,
                                            overflow: 'hidden',
                                            minHeight: 22,
                                        })}
                                    >
                                        <Typography
                                            component="span"
                                            sx={(theme) => ({
                                                fontSize: '0.6rem',
                                                fontWeight: 700,
                                                color: getRouteGraphActiveColor(theme),
                                                flexShrink: 0,
                                                lineHeight: 1,
                                            })}
                                        >
                                            {spec.label}
                                        </Typography>
                                        {(displayVal || spec.type === 'int') && (
                                            <Typography
                                                component="span"
                                                sx={{
                                                    fontSize: '0.6rem',
                                                    fontWeight: 500,
                                                    color: 'text.primary',
                                                    overflow: 'hidden',
                                                    textOverflow: 'ellipsis',
                                                    whiteSpace: 'nowrap',
                                                    flexGrow: 1,
                                                    lineHeight: 1,
                                                }}
                                            >
                                                : {spec.type === 'int' ? formatSeconds(flagIntValue(flags, spec.key)) : displayVal}
                                            </Typography>
                                        )}
                                        {spec.type === 'bool' && onToggleFlag && (
                                            <IconButton
                                                size="small"
                                                onClick={(e) => { e.stopPropagation(); onToggleFlag(spec.key); }}
                                                sx={{ p: 0, ml: 'auto', flexShrink: 0, color: 'text.disabled', '&:hover': { color: 'error.main' } }}
                                            >
                                                <CloseIcon sx={{ fontSize: '0.7rem' }} />
                                            </IconButton>
                                        )}
                                    </Box>
                                </Tooltip>
                            );
                        })}
                    </Stack>
                </Box>
            )}
        </StyledExtensionsCard>
    );
};

export default RuleExtensionsCard;
