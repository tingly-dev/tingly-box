import {Box, Tooltip, useTheme} from '@mui/material';
import {alpha, decomposeColor} from '@mui/material/styles';
import type {SxProps, Theme} from '@mui/material';
import { EMPTY_SX } from '@/constants/defaults';
import { AnthropicStyleMark, GoogleStyleMark, OpenAIStyleMark } from '@/components/icons';

interface ApiStyleBadgeProps {
    apiStyle: string;
    compact?: boolean;
    minimal?: boolean;
    minimalSize?: 'small' | 'medium';
    sx?: SxProps<Theme>;
}

type Rgb = { r: number; g: number; b: number };

// Resolve any CSS color MUI understands (hex / rgb() / rgba() / hsl() / names)
// into an opaque {r,g,b} triple. rgba() alpha is pre-composited over opaque
// white so the result is always fully opaque — this is what makes the badge
// background solid even when the theme's paper is itself translucent (e.g. the
// sunlit theme uses rgba(255,255,255,0.75) paper).
const toOpaqueRgb = (color: string): Rgb | null => {
    const c = decomposeColor(color);
    const [r, g, b, a = 1] = c.values as number[];
    if ([r, g, b].some((n) => !Number.isFinite(n))) return null;
    const alphaV = Number.isFinite(a) ? a : 1;
    // Composite onto white if there's an alpha channel.
    const over = (n: number) => Math.round(n * alphaV + 255 * (1 - alphaV));
    return { r: over(r), g: over(g), b: over(b) };
};

// Composite a translucent tint over an opaque paper base → one opaque color.
const blend = (tint: string, tintAlpha: number, paperRgb: Rgb): string => {
    const t = toOpaqueRgb(tint);
    if (!t) return `rgb(${paperRgb.r}, ${paperRgb.g}, ${paperRgb.b})`;
    const mix = (n: number, p: number) => Math.round(n * tintAlpha + p * (1 - tintAlpha));
    return `rgb(${mix(t.r, paperRgb.r)}, ${mix(t.g, paperRgb.g)}, ${mix(t.b, paperRgb.b)})`;
};

// Helper function to render API style badge with icon and colored background
export const ApiStyleBadge = ({
    apiStyle,
    sx = EMPTY_SX,
    compact = false,
    minimal = false,
    minimalSize = 'small',
}: ApiStyleBadgeProps) => {
    const theme = useTheme();
    const isOpenAI = apiStyle === 'openai';
    const isAnthropic = apiStyle === 'anthropic';
    const isGoogle = apiStyle === 'google';

    if (!isOpenAI && !isAnthropic && !isGoogle) {
        return null; // Don't show badge for unknown styles
    }

    // The badge floats (position:absolute) over the service node's own text
    // (provider name), so its background must be FULLY OPAQUE — any alpha lets
    // that text bleed through. We composite each brand tint onto the theme's
    // paper color to produce a solid color per mode rather than a transparent
    // wash. Mirrors the solid-paper backing used by ActionButtonsBox.
    const isDark = theme.palette.mode === 'dark';
    const paperRgb = toOpaqueRgb(theme.palette.background.paper) ?? { r: 255, g: 255, b: 255 };

    // One table per provider: identity + brand color + tint strength.
    // fill is the resting tint opacity over paper; hoverFill is the stronger
    // hover opacity. Dark mode gets a higher fill so tints stay visible on dark paper.
    const providers = {
        openai:    { label: 'OpenAI',    Mark: OpenAIStyleMark, tint: theme.palette.info.main, fill: isDark ? 0.22 : 0.14, hoverFill: isDark ? 0.32 : 0.22, border: alpha(theme.palette.info.main, 0.4) },
        anthropic: { label: 'Anthropic', Mark: AnthropicStyleMark, tint: '#E07A5F',               fill: isDark ? 0.26 : 0.16, hoverFill: isDark ? 0.36 : 0.26, border: alpha('#E07A5F', 0.5) },
        google:    { label: 'Google',    Mark: GoogleStyleMark, tint: '#4285F4',               fill: isDark ? 0.22 : 0.14, hoverFill: isDark ? 0.32 : 0.22, border: alpha('#4285F4', 0.4) },
    } as const;
    const p = isOpenAI ? providers.openai : isAnthropic ? providers.anthropic : providers.google;

    const backgroundColor = blend(p.tint, p.fill, paperRgb); // opaque — no bleed-through
    const hoverBackgroundColor = blend(p.tint, p.hoverFill, paperRgb);

    if (minimal) {
        const diameter = minimalSize === 'medium' ? 20 : 16;
        const ProtocolMark = p.Mark;

        return (
            <Tooltip title={`${p.label} Style`} arrow placement="top">
                <Box
                    sx={{
                        width: diameter,
                        height: diameter,
                        borderRadius: '50%',
                        display: 'inline-flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        flexShrink: 0,
                        color: p.tint,
                        '& .api-style-mark-frame': {
                            fill: backgroundColor,
                            stroke: p.border,
                        },
                        '& .api-style-mark-glyph': {
                            stroke: p.tint,
                        },
                        ...sx,
                    }}
                >
                    <ProtocolMark aria-hidden sx={{ width: '100%', height: '100%', display: 'block' }} />
                </Box>
            </Tooltip>
        );
    }

    return (
        <Box
            sx={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 0.5,
                px: compact ? 0.65 : 1.1,
                py: compact ? 0.125 : 0.375,
                borderRadius: theme.shape.borderRadius,
                fontSize: compact ? '8px' : '10px',
                fontWeight: 600,
                height: compact ? '16px' : '20px',
                minWidth: compact ? 'unset' : '76px',
                border: `1px solid ${p.border}`,
                backgroundColor,
                color: p.tint,
                transition: theme.transitions.create(['background-color', 'color', 'border-color'], {
                    duration: theme.transitions.duration.shorter,
                }),
                '&:hover': {
                    backgroundColor: hoverBackgroundColor,
                    borderColor: alpha(p.tint, 0.6),
                },
                ...sx,
            }}
        >
            <span>{p.label}{compact ? '' : ' Style'}</span>
        </Box>
    );
};
