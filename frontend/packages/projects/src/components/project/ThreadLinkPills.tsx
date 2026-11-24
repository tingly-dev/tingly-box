import { useState } from 'react';
import { Box, ClickAwayListener, Link, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { ThreadLink } from '../../client';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { colorOf, keyParts, LinkIcon, linkParts, stripLinks } from './linkView';

const pill = {
    display: 'inline-flex', alignItems: 'center', gap: 0.5, height: 22, px: 0.75, borderRadius: radius.chip, flexShrink: 0,
    fontSize: fontSizes.sm, fontWeight: 500, whiteSpace: 'nowrap', border: 1, borderColor: 'divider', color: 'text.secondary',
    textDecoration: 'none', cursor: 'pointer', '&:hover': { borderColor: 'text.disabled', color: 'text.primary' },
    '&:focus-visible': { outline: 'none', boxShadow: (th: { palette: { primary: { main: string } } }) => `0 0 0 2px ${th.palette.primary.main}` },
    '& svg': { fontSize: 13, mt: 0 },
} as const;

const href = (l: ThreadLink) => (l.url.startsWith('https://') ? l.url : undefined);
const linkKey = (l: ThreadLink) => `${l.repo}/${l.kind}/${l.number}`;

/** What is wrong with an item that the user may have to act on: checks failing or a conflict. Draws as a red dot on its pill. */
export const linkTrouble = (l: ThreadLink) => l.state === 'open' && (l.ci === 'failing' || l.merge_state === 'conflicts');

/** The words of an item in one line: "owner/repo#12 Title · Open · checks failing". */
const describe = (l: ThreadLink, t: (key: string) => string) => [`${l.repo}#${l.number} ${l.title}`, ...linkParts(l, t).map((p) => p.text)].join(' · ');

// The row a pill sits in opens the thread: what is clicked on a pill stays on the pill.
const stop = (e: { stopPropagation(): void }) => e.stopPropagation();

/** One item as a pill: its kind and state in the icon's shape and colour, its number, a red dot when it needs attention. Opens it on GitHub. */
function LinkPill({ link }: { link: ThreadLink }) {
    const { t } = useTranslation('projects');
    const words = describe(link, t);
    return (
        <Box component="a" href={href(link)} target="_blank" rel="noopener noreferrer" title={words} aria-label={words}
            data-link-pill={`${link.kind}-${link.number}`} onClick={stop} onKeyDown={stop} sx={pill}>
            <LinkIcon link={link} />#{link.number}
            {linkTrouble(link) && <Box component="span" aria-hidden sx={{ width: 6, height: 6, borderRadius: '50%', bgcolor: (th) => colorOf(th, 'error') }} />}
        </Box>
    );
}

/** Every item, one line each, for the `+N` pill's card. */
function LinkList({ links }: { links: ThreadLink[] }) {
    const { t } = useTranslation('projects');
    return (
        <Box sx={{ py: 0.25, maxWidth: 360 }}>
            {links.map((l) => (
                <Box key={linkKey(l)} sx={{ display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0, py: 0.25, '& svg': { mt: 0 } }}>
                    <LinkIcon link={l} />
                    <Link href={href(l)} target="_blank" rel="noopener noreferrer" underline="hover" color="text.primary" title={`${l.repo}#${l.number} ${l.title}`}
                        sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: fontSizes.sm }}>
                        #{l.number} {l.title}
                    </Link>
                    {keyParts(l, t).map((p) => <Box key={p.text} component="span" sx={{ flexShrink: 0, fontSize: fontSizes.sm, color: (th) => colorOf(th, p.tone) }}>{p.text}</Box>)}
                </Box>
            ))}
        </Box>
    );
}

/**
 * A thread's issues and PRs on its card (the Overview's boards, the band's
 * columns), compact: the first one (an open
 * PR before an open issue before what finished, as the composer strip orders
 * them) as a pill, the rest behind `+N`, whose card lists them all on hover
 * or click. Read from `Thread.links`, which the thread list and its live
 * events already carry: no request of its own. Nothing without a link.
 */
export function ThreadLinkPills({ links }: { links: ThreadLink[] | undefined }) {
    const { t } = useTranslation('projects');
    const [open, setOpen] = useState(false);
    const list = stripLinks(links);
    if (list.length === 0) return null;
    const [first, ...rest] = list;
    const more = rest.length > 0 && (
        <ClickAwayListener onClickAway={() => setOpen(false)}>
            <span>
                <Tooltip open={open} onOpen={() => setOpen(true)} onClose={() => setOpen(false)} title={<LinkList links={list} />}
                    slotProps={{ tooltip: { onClick: stop, sx: { bgcolor: 'background.paper', color: 'text.primary', boxShadow: 3, border: 1, borderColor: 'divider', maxWidth: 380 } } }}>
                    <Box component="button" type="button" aria-label={t('links.titleCount', { count: list.length })} aria-expanded={open}
                        data-link-more onClick={(e) => { stop(e); setOpen(true); }} onKeyDown={stop}
                        sx={{ font: 'inherit', bgcolor: 'transparent', ...pill }}>
                        +{rest.length}
                        {rest.some(linkTrouble) && <Box component="span" aria-hidden sx={{ width: 6, height: 6, borderRadius: '50%', bgcolor: (th) => colorOf(th, 'error') }} />}
                    </Box>
                </Tooltip>
            </span>
        </ClickAwayListener>
    );
    return (
        <>
            <LinkPill link={first} />
            {more}
        </>
    );
}
