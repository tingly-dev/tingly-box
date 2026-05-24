import { Add as AddIcon, Extension as ExtensionIcon } from '@/components/icons';
import { Box, Chip, Stack, Tooltip, Typography } from '@mui/material';
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
        default:
            return false;
    }
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
        default:
            return '';
    }
};

// Enum default that should be treated as "unset" (registry's first option).
const ENUM_DEFAULT_VALUE = 'auto';

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
        if (spec.type === 'enum') {
            const v = flagStringValue(flags, spec.key);
            return v !== '' && v !== ENUM_DEFAULT_VALUE;
        }
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
                    <Stack direction="row" flexWrap="wrap" gap={0.5}>
                        {enabled.map((spec) => {
                            const isString = spec.type === 'string';
                            const isEnum = spec.type === 'enum';
                            const stringVal = isString || isEnum ? flagStringValue(flags, spec.key) : '';
                            let displayVal = stringVal;
                            if (isEnum) {
                                const opt = (spec.options || []).find((o) => o.value === stringVal);
                                if (opt) displayVal = opt.label;
                            }
                            const label = (isString || isEnum) && displayVal
                                ? `${spec.label}: ${displayVal}`
                                : spec.label;
                            const title = (isString || isEnum) && stringVal
                                ? `${spec.description}\nValue: ${displayVal}`
                                : spec.description;
                            return (
                                <Tooltip key={spec.key} title={title}>
                                    <Chip
                                        size="small"
                                        label={label}
                                        variant="outlined"
                                        onDelete={
                                            spec.type === 'bool' && onToggleFlag
                                                ? (e: React.MouseEvent) => {
                                                    // Don't let the X bubble into the
                                                    // card-level "open catalog" handler.
                                                    e.stopPropagation();
                                                    onToggleFlag(spec.key);
                                                }
                                                : undefined
                                        }
                                        sx={(theme) => ({
                                            maxWidth: '100%',
                                            height: 20,
                                            fontSize: '0.65rem',
                                            borderColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.55 : 0.48),
                                            color: getRouteGraphActiveColor(theme),
                                            backgroundColor: alpha(getRouteGraphActiveColor(theme), theme.palette.mode === 'dark' ? 0.08 : 0.035),
                                            '& .MuiChip-label': {
                                                px: 0.75,
                                            },
                                            '& .MuiChip-deleteIcon': {
                                                color: alpha(getRouteGraphActiveColor(theme), 0.72),
                                                fontSize: 15,
                                                '&:hover': {
                                                    color: getRouteGraphActiveColor(theme),
                                                },
                                            },
                                        })}
                                    />
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
