/**
 * ToolCard
 *
 * A reusable card component for displaying a single tool (MCP tool, virtual tool, etc.)
 * Matches the design: enabled = green border + shadow, disabled = grey border.
 *
 * Usage:
 *   <ToolCard
 *     icon={<IconBolt />}
 *     name="web_search"
 *     description="Browser-side search via Serper."
 *     enabled={true}
 *     badges={[{ label: 'Client', color: 'blue' }]}
 *     tags={['mcp_web_search']}
 *     onToggle={(enabled) => ...}
 *     settings={<>...config fields...</>}
 *   />
 */

import React from 'react';
import { Box, Switch, Typography } from '@mui/material';

// ─── Types ────────────────────────────────────────────────────────────────────

export type BadgeColor = 'blue' | 'green' | 'orange' | 'gray';

export interface ToolCardBadge {
    label: string;
    color: BadgeColor;
}

export interface ToolCardProps {
    /** Icon element rendered in the 36×36 icon area */
    icon: React.ReactNode;
    /** Tool name (heading) */
    name: string;
    /** Short description below the name */
    description: string;
    /** Whether the tool is currently enabled */
    enabled: boolean;
    /** Callback when the toggle is flipped */
    onToggle?: (enabled: boolean) => void;
    /** Small badge chips shown next to the name (e.g. "Client", "Experimental") */
    badges?: ToolCardBadge[];
    /** Tool name chips shown as monospace tags */
    tags?: string[];
    /** Optional config area shown below the description when expanded */
    settings?: React.ReactNode;
    /** Disable the toggle (e.g. while saving) */
    toggleDisabled?: boolean;
    /** Disable click-to-expand behavior */
    noExpand?: boolean;
}

// ─── Badge color map ──────────────────────────────────────────────────────────

const BADGE_STYLES: Record<BadgeColor, { bg: string; color: string }> = {
    blue: { bg: 'rgb(238, 242, 255)', color: 'rgb(30, 64, 175)' },
    green: { bg: 'rgba(10, 124, 90, 0.1)', color: 'rgb(10, 124, 90)' },
    orange: { bg: 'rgb(255, 247, 237)', color: 'rgb(194, 65, 12)' },
    gray: { bg: 'rgb(241, 243, 246)', color: 'rgb(107, 114, 128)' },
};

// ─── Sub-components ──────────────────────────────────────────────────────────

const Badge: React.FC<{ label: string; color: BadgeColor }> = ({ label, color }) => {
    const s = BADGE_STYLES[color];
    return (
        <Box
            component="span"
            sx={{
                display: 'inline-flex',
                alignItems: 'center',
                height: 18,
                px: '6px',
                py: '2px',
                bgcolor: s.bg,
                color: s.color,
                borderRadius: '6px',
                border: '1px solid',
                borderColor: s.color,
                fontSize: '0.625rem',
                fontWeight: 600,
                letterSpacing: '-0.05px',
                lineHeight: 1,
                flexShrink: 0,
            }}
        >
            {label}
        </Box>
    );
};

const Tag: React.FC<{ label: string }> = ({ label }) => (
    <Box
        component="span"
        sx={{
            display: 'inline-flex',
            alignItems: 'center',
            height: 18,
            px: '6px',
            py: '2px',
            bgcolor: 'action.selected',
            color: 'rgb(75, 85, 99)',
            borderRadius: '4px',
            fontSize: '0.65rem',
            fontWeight: 500,
            fontFamily: 'monospace',
            lineHeight: 1,
            flexShrink: 0,
        }}
    >
        {label}
    </Box>
);

// ─── Main component ──────────────────────────────────────────────────────────

export const ToolCard: React.FC<ToolCardProps> = ({
    icon,
    name,
    description,
    enabled,
    onToggle,
    badges = [],
    tags = [],
    settings,
    toggleDisabled = false,
    noExpand = false,
}) => {
    const [expanded, setExpanded] = React.useState(false);

    const handleCardClick = () => {
        if (!noExpand && settings) setExpanded((v) => !v);
    };

    return (
        <Box
            onClick={handleCardClick}
            sx={{
                bgcolor: 'background.paper',
                borderRadius: '12px',
                border: '1px solid',
                borderColor: enabled ? 'rgb(10, 124, 90)' : 'rgb(229, 231, 236)',
                boxShadow: enabled ? 'rgba(10, 124, 90, 0.08) 0px 0px 0px 3px' : 'none',
                p: '14px 16px',
                cursor: (settings && !noExpand) ? 'pointer' : 'default',
                transition: 'border-color 0.2s, box-shadow 0.2s',
            }}
        >
            {/* ── Header row ── */}
            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1.5 }}>
                {/* Icon */}
                <Box
                    sx={{
                        width: 36,
                        height: 36,
                        borderRadius: '9px',
                        bgcolor: enabled ? 'rgba(10, 124, 90, 0.08)' : 'rgb(241, 243, 246)',
                        color: enabled ? 'rgb(10, 124, 90)' : 'rgb(107, 114, 128)',
                        display: 'grid',
                        placeItems: 'center',
                        flexShrink: 0,
                        transition: 'background-color 0.2s, color 0.2s',
                    }}
                >
                    {icon}
                </Box>

                {/* Name + badges + tags */}
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 0.75, mb: 0.5 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 700, lineHeight: 1.2 }}>
                            {name}
                        </Typography>
                        {badges.map((b) => (
                            <Badge key={b.label} label={b.label} color={b.color} />
                        ))}
                        {tags.map((t) => (
                            <Tag key={t} label={t} />
                        ))}
                    </Box>
                    <Typography variant="caption" color="text.secondary" sx={{ lineHeight: 1.4 }}>
                        {description}
                    </Typography>
                </Box>

                {/* Toggle */}
                {onToggle && (
                    <Box
                        onClick={(e) => e.stopPropagation()}
                        sx={{ flexShrink: 0, display: 'flex', alignItems: 'center' }}
                    >
                        <Switch
                            size="small"
                            checked={enabled}
                            onChange={(e) => onToggle(e.target.checked)}
                            disabled={toggleDisabled}
                        />
                    </Box>
                )}
            </Box>

            {/* ── Expanded settings ── */}
            {settings && expanded && (
                <Box
                    onClick={(e) => e.stopPropagation()}
                    sx={{ mt: 2, pt: 2, borderTop: '1px solid', borderColor: 'divider' }}
                >
                    {settings}
                </Box>
            )}
        </Box>
    );
};

export default ToolCard;
