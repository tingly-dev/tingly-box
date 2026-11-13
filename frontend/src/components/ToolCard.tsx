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
import { toneColors, type Tone } from '@tingly/ui/theme/tone';
import { Box, Switch, Typography } from '@mui/material';
import { fontMono, fontSizes } from '@/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

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
    /** Always-visible action buttons shown in the header row (e.g. edit/delete) */
    actions?: React.ReactNode;
    /** Disable the toggle (e.g. while saving) */
    toggleDisabled?: boolean;
    /** Disable click-to-expand behavior */
    noExpand?: boolean;
    /** Start expanded (e.g. when configuration is missing) */
    defaultExpanded?: boolean;
    /** Controlled expanded state — overrides internal toggle when provided */
    expanded?: boolean;
}

// ─── Badge color map ──────────────────────────────────────────────────────────

const BADGE_TONES: Record<BadgeColor, Tone> = {
    blue: 'primary',
    green: 'success',
    orange: 'warning',
    gray: 'neutral',
};

// ─── Sub-components ──────────────────────────────────────────────────────────

const Badge: React.FC<{ label: string; color: BadgeColor }> = ({ label, color }) => {
    return (
        <Box
            component="span"
            sx={(theme) => {
                const c = toneColors(theme, BADGE_TONES[color]);
                return {
                    display: 'inline-flex',
                    alignItems: 'center',
                    height: 18,
                    px: '6px',
                    py: '2px',
                    bgcolor: c.bg,
                    color: c.ink,
                    borderRadius: radius.chip,
                    border: '1px solid',
                    borderColor: c.line,
                    fontSize: fontSizes.micro,
                    fontWeight: 600,
                    letterSpacing: '-0.05px',
                    lineHeight: 1,
                    flexShrink: 0,
                };
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
            color: 'text.secondary',
            borderRadius: radius.chip,
            fontSize: fontSizes.micro,
            fontWeight: 500,
            fontFamily: fontMono,
            lineHeight: 1,
            flexShrink: 0,
        }}
    >
        {label}
    </Box>
);

// ─── Main component ──────────────────────────────────────────────────────────

const EMPTY_BADGES: ToolCardBadge[] = [];
const EMPTY_TAGS: string[] = [];

export const ToolCard: React.FC<ToolCardProps> = ({
    icon,
    name,
    description,
    enabled,
    onToggle,
    badges = EMPTY_BADGES,
    tags = EMPTY_TAGS,
    settings,
    actions,
    toggleDisabled = false,
    noExpand = false,
    defaultExpanded = false,
    expanded: controlledExpanded,
}) => {
    const [internalExpanded, setInternalExpanded] = React.useState(defaultExpanded);
    const expanded = controlledExpanded !== undefined ? controlledExpanded : internalExpanded;

    const handleCardClick = () => {
        if (!noExpand && settings) setInternalExpanded((v) => !v);
    };

    return (
        <Box
            onClick={handleCardClick}
            sx={(theme) => {
                const on = toneColors(theme, 'success');
                return {
                    bgcolor: 'background.paper',
                    borderRadius: radius.card,
                    border: '1px solid',
                    borderColor: enabled ? on.line : 'divider',
                    boxShadow: enabled ? `0 0 0 3px ${on.ring}` : 'none',
                    p: '14px 16px',
                    cursor: (settings && !noExpand) ? 'pointer' : 'default',
                    transition: 'border-color 0.2s, box-shadow 0.2s',
                };
            }}
        >
            {/* ── Header row ── */}
            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1.5 }}>
                {/* Icon */}
                <Box
                    sx={(theme) => {
                        const c = toneColors(theme, enabled ? 'success' : 'neutral');
                        return {
                            width: 36,
                            height: 36,
                            borderRadius: radius.row,
                            bgcolor: c.bg,
                            color: c.ink,
                            display: 'grid',
                            placeItems: 'center',
                            flexShrink: 0,
                            transition: 'background-color 0.2s, color 0.2s',
                        };
                    }}
                >
                    {icon}
                </Box>

                {/* Name + badges + tags */}
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 0.75, mb: 0.5 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                            {name}
                        </Typography>
                        {badges.map((b) => (
                            <Badge key={b.label} label={b.label} color={b.color} />
                        ))}
                        {tags.map((t) => (
                            <Tag key={t} label={t} />
                        ))}
                    </Box>
                    <Typography
                        variant="caption"
                        sx={{
                            color: "text.secondary",
                            lineHeight: 1.4
                        }}>
                        {description}
                    </Typography>
                </Box>

                {/* Actions (always visible) */}
                {actions && (
                    <Box
                        onClick={(e) => e.stopPropagation()}
                        sx={{ flexShrink: 0, display: 'flex', alignItems: 'center' }}
                    >
                        {actions}
                    </Box>
                )}

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
