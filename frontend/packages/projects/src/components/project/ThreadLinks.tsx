import { useState } from 'react';
import { Box, Link } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ThreadLink } from '../../client';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { colorOf, LinkIcon, linkParts, stripLinks } from './linkView';

const linkKey = (l: ThreadLink) => `${l.repo}/${l.kind}/${l.number}`;

/** Items a thread's list row shows before folding the rest; a fold that would hide a single item is no fold. */
const ROWS_SHOWN = 3;

// The row these lines sit in opens the thread: what is clicked here stays here.
const stop = (e: { stopPropagation(): void }) => e.stopPropagation();

/**
 * A thread's issues and PRs under its row in the list view, one line each, as
 * the strip above the message box orders them (an open PR, then an open
 * issue, then what finished): kind/state icon, `#214 title` as a link to
 * GitHub, and where it stands (open or draft, checks, mergeability). The list
 * is where the whole picture is read, so nothing is folded into a pill; past
 * `ROWS_SHOWN` (and only when that hides two or more) the rest wait behind
 * "Show N more", which opens in place. Read from `Thread.links`, which the
 * thread list and its live events already carry: no request of its own.
 */
export function ThreadLinkRows({ links }: { links: ThreadLink[] | undefined }) {
    const { t } = useTranslation('projects');
    const [all, setAll] = useState(false);
    const list = stripLinks(links);
    if (list.length === 0) return null;
    const folds = list.length > ROWS_SHOWN + 1;
    const shown = folds && !all ? list.slice(0, ROWS_SHOWN) : list;
    return (
        <Box data-link-rows onClick={stop} onKeyDown={stop} sx={{ mt: 0.5, ml: 0.5, pl: 1, borderLeft: 1, borderColor: 'divider', fontSize: fontSizes.sm, lineHeight: 1.4 }}>
            {shown.map((l) => (
                <Box key={linkKey(l)} data-link-row={`${l.kind}-${l.number}`}
                    sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', columnGap: 0.75, minWidth: 0, minHeight: 24, '& svg': { mt: 0, fontSize: 14 } }}>
                    <LinkIcon link={l} />
                    <Link href={l.url.startsWith('https://') ? l.url : undefined} target="_blank" rel="noopener noreferrer" underline="hover" color="text.primary" title={`${l.repo}#${l.number} ${l.title}`}
                        sx={{ flex: '1 1 7rem', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        #{l.number} {l.title}
                    </Link>
                    {linkParts(l, t).map((p) => <Box key={p.text} component="span" sx={{ flexShrink: 0, color: (th) => colorOf(th, p.tone) }}>{p.text}</Box>)}
                </Box>
            ))}
            {folds && (
                <Link component="button" type="button" underline="hover" color="text.secondary" aria-expanded={all} onClick={() => setAll((a) => !a)}
                    sx={{ minHeight: 24, fontSize: 'inherit' }}>
                    {all ? t('links.less') : t('links.more', { count: list.length - ROWS_SHOWN })}
                </Link>
            )}
        </Box>
    );
}
