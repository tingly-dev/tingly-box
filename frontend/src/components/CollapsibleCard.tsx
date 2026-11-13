import { Box, ButtonBase, Collapse, Typography } from '@mui/material';
import type { ReactNode } from 'react';
import { ExpandMore } from '@/components/icons';
import UnifiedCard from '@/components/UnifiedCard.tsx';
import { radius } from '@tingly/ui/theme/radius';

interface CollapsibleCardProps {
    title: string;
    /** One-line summary, always visible whether the card is open or not. */
    description?: string;
    expanded: boolean;
    onToggle: () => void;
    /** Cap the *collapsed body*'s width only — the header/toggle always
     * spans the full card so the chevron sits flush at the true edge. */
    contentMaxWidth?: number | string;
    /** Cap the body's height with an internal scrollbar, for a section whose
     * content can grow arbitrarily long (e.g. a provider catalog) — so one
     * card can't push every other section on the page below the fold. */
    contentMaxHeight?: number | string;
    children: ReactNode;
}

/**
 * UnifiedCard shell with an accordion-style header: title + one-line
 * description stay visible either way, the body hides behind a chevron.
 * Lets a page hold several differently-sized cards side by side without all
 * of them being fully open (and fighting for attention) at once.
 *
 * The header is rendered as plain card content, not via UnifiedCard's
 * `title` prop — that slot is a flex item sized to its own content (it was
 * built for a short label + optional icon), so a full-width, space-between
 * header dropped in there would hug its own text instead of reaching the
 * card's real right edge. Rendering it as the first child keeps it inside
 * UnifiedCard's content box, which does stretch to the full card width.
 *
 * State (open/closed) is fully controlled by the caller — this only renders.
 */
export const CollapsibleCard = ({ title, description, expanded, onToggle, contentMaxWidth, contentMaxHeight, children }: CollapsibleCardProps) => {
    return (
        <UnifiedCard size="full">
            <ButtonBase
                onClick={onToggle}
                aria-expanded={expanded}
                sx={{
                    width: '100%',
                    display: 'flex',
                    alignItems: 'flex-start',
                    justifyContent: 'space-between',
                    gap: 2,
                    textAlign: 'left',
                    borderRadius: radius.row,
                    p: 0.5,
                    m: -0.5,
                    '&:hover': { bgcolor: 'action.hover' },
                }}
            >
                <Box sx={{ minWidth: 0 }}>
                    <Typography variant="h4" sx={{ fontWeight: 600, color: 'text.primary' }}>
                        {title}
                    </Typography>
                    {description && (
                        <Typography variant="body2" sx={{ color: 'text.secondary', mt: 0.5 }}>
                            {description}
                        </Typography>
                    )}
                </Box>
                <ExpandMore
                    sx={{
                        flexShrink: 0,
                        mt: 0.5,
                        color: 'text.secondary',
                        transform: expanded ? 'rotate(180deg)' : 'none',
                        transition: 'transform 0.2s ease',
                    }}
                />
            </ButtonBase>

            <Collapse in={expanded} timeout="auto">
                <Box
                    sx={{
                        pt: 2,
                        ...(contentMaxWidth !== undefined && { maxWidth: contentMaxWidth }),
                        ...(contentMaxHeight !== undefined && { maxHeight: contentMaxHeight, overflowY: 'auto' }),
                    }}
                >
                    {children}
                </Box>
            </Collapse>
        </UnifiedCard>
    );
};

export default CollapsibleCard;
