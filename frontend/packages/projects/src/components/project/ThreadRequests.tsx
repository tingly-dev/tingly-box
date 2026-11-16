// A thread that waits on an approval or a question shows it on its card and
// answers it there. The card's subtitle says what is asked (the command, the
// question); under it, one quiet row opens the request in a popover with the
// same card the thread page shows, and a way into the thread for the full
// context. Several pending requests are listed together, oldest first.
import { useEffect, useRef, useState, type MouseEvent } from 'react';
import { Box, Button, Popover, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import type { Thread } from '../../client';
import { useThreadRequests, useThreadRequestsContext, type PendingRequest } from '../../hooks/threadRequests';
import { useNotify } from '../../hooks/useNotify';
import { PendingRequestCard, TranscriptHostProvider, approvalSubject, askQuestions, type Respond } from '../../transcript';
import { Info, OpenInNew } from '../icons';
import { DisclosureRow } from '../common/Disclosure';
import { ThreadStateMark, ThreadSubtitle } from './threadStatus';

/** How long a popover that has nothing left to answer stays before it closes itself. */
export const CLEARED_CLOSE_MS = 1600;

const firstLine = (s: string) => s.split('\n').find((l) => l.trim() !== '') ?? '';

/** What one request asks, as a line: the tool and its command or path, or the question. */
function RequestLine({ request }: { request: PendingRequest }) {
    const { entry } = request;
    if (request.kind === 'ask') {
        const question = askQuestions(entry.payload)[0]?.question || entry.content;
        return <>{firstLine(question)}</>;
    }
    const subject = approvalSubject(entry.content, entry.payload).text;
    return (
        <>
            <Box component="span" sx={{ fontWeight: 600, color: 'text.primary' }}>{entry.content}</Box>
            {subject && <Box component="span" sx={{ fontFamily: fontMono, fontSize: fontSizes.inlineCode, ml: 0.75 }}>{firstLine(subject)}</Box>}
        </>
    );
}

/**
 * The line under a card's title: what its pending request asks, else what the
 * thread usually says there. The wait reason it replaces only paraphrases the
 * request.
 */
export function ThreadRequestSubtitle({ thread }: { thread: Thread }) {
    const { t } = useTranslation('projects');
    const requests = useThreadRequests(thread.id);
    if (requests.length === 0) return <ThreadSubtitle thread={thread} />;
    return (
        <>
            <RequestLine request={requests[0]} />
            {requests.length > 1 && <Box component="span" sx={{ ml: 0.75, color: 'text.disabled' }}>{t('thread.requests.more', { count: requests.length - 1 })}</Box>}
        </>
    );
}

function labelOf(t: (key: string, values?: Record<string, unknown>) => string, requests: PendingRequest[]) {
    if (requests.length === 0) return t('thread.requests.answered');
    if (requests.length > 1) return t('thread.requests.many', { count: requests.length });
    return requests[0].kind === 'ask' ? t('thread.requests.question') : t('thread.requests.approval');
}

/**
 * The row under a waiting thread's card that opens its requests. Nothing
 * where the thread waits on nothing it can be answered for (a failed turn,
 * a report that needs a reply): there the card opens the thread as before.
 * `onOpen` opens the thread itself.
 */
export function ThreadRequestsStrip({ thread, onOpen }: { thread: Thread; onOpen: (id: string) => void }) {
    const { t } = useTranslation('projects');
    const ctx = useThreadRequestsContext();
    const requests = ctx?.byThread[thread.id] ?? [];
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const ref = useRef<HTMLDivElement>(null);
    const open = anchor != null;

    // A notification's deep link: the card whose requests were asked for opens them.
    const reviewing = ctx?.reviewId === thread.id && requests.length > 0;
    useEffect(() => {
        if (!reviewing || !ref.current) return;
        ref.current.scrollIntoView?.({ block: 'nearest' });
        setAnchor(ref.current);
        ctx?.reviewed();
    }, [reviewing, ctx]);

    if (!ctx || (requests.length === 0 && !open)) return null;
    // The card is a button too: nothing in here may reach it.
    const keep = (e: MouseEvent) => e.stopPropagation();
    return (
        <Box ref={ref} onClick={keep} sx={{ mt: 0.75 }}>
            <DisclosureRow
                open={open}
                onToggle={() => setAnchor(open ? null : ref.current)}
                aria-haspopup="dialog"
                aria-label={`${labelOf(t, requests)} · ${t('thread.requests.review')}`}
                sx={{ border: 1, borderColor: 'divider', borderRadius: radius.row, px: 1, py: 0.5, fontSize: fontSizes.sm }}
            >
                <ThreadStateMark status="waiting" />
                <Box component="span" sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: 500, color: 'text.primary' }}>{labelOf(t, requests)}</Box>
                <Box component="span" sx={{ flexShrink: 0 }}>{t('thread.requests.review')}</Box>
            </DisclosureRow>
            <RequestsPopover thread={thread} requests={requests} anchor={anchor} onClose={() => setAnchor(null)} onOpenThread={onOpen} />
        </Box>
    );
}

/**
 * The pending requests of one thread as the thread page's own cards, answered
 * in place. `onSettled` runs when a request turned out to be answered
 * elsewhere already.
 */
export function RequestCards({ thread, requests, onSettled }: { thread: Thread; requests: PendingRequest[]; onSettled?: () => void }) {
    const { t } = useTranslation('projects');
    const ctx = useThreadRequestsContext();
    const notify = useNotify();

    const respond: Respond = async (requestId, allow, answer, remember, answers) => {
        try {
            const result = await ctx!.respond(thread.id, requestId, allow, answer, remember, answers);
            if (result === 'settled') onSettled?.();
            return true;
        } catch (e) {
            notify((e as Error).message, 'error');
            return false;
        }
    };

    return (
        <TranscriptHostProvider rememberLabel={(tool) => t('thread.allowThread', { tool })}>
            <Box sx={{ display: 'grid', gap: 1 }}>
                {requests.map((r) => (
                    <PendingRequestCard key={r.id} block={{ type: 'request', message: r.entry }} onRespond={respond} />
                ))}
            </Box>
        </TranscriptHostProvider>
    );
}

function RequestsPopover({ thread, requests, anchor, onClose, onOpenThread }: {
    thread: Thread; requests: PendingRequest[]; anchor: HTMLElement | null; onClose: () => void; onOpenThread: (id: string) => void;
}) {
    const { t } = useTranslation('projects');
    const [settled, setSettled] = useState(false);
    const open = anchor != null;

    // Everything was answered (here, or elsewhere): say so, then get out of the way.
    const empty = requests.length === 0;
    useEffect(() => {
        if (!open || !empty) return;
        const timer = setTimeout(onClose, CLEARED_CLOSE_MS);
        return () => clearTimeout(timer);
    }, [open, empty, onClose]);

    return (
        <Popover
            open={open}
            anchorEl={anchor}
            onClose={onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            slotProps={{ paper: { sx: { width: 440, maxWidth: '92vw', maxHeight: '75vh' } }, transition: { onExited: () => setSettled(false) } }}
        >
            <Box role="dialog" aria-label={t('thread.requests.dialog', { title: thread.title })} sx={{ p: 1.25 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0, mb: 1 }}>
                    <ThreadStateMark status="waiting" />
                    <Typography noWrap title={thread.title} sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.base, fontWeight: 600, color: 'text.primary' }}>{thread.title}</Typography>
                    <Button size="small" color="inherit" endIcon={<OpenInNew sx={{ fontSize: 14 }} />} sx={{ flexShrink: 0, color: 'text.secondary' }}
                        onClick={() => { onClose(); onOpenThread(thread.id); }}>
                        {t('thread.requests.openThread')}
                    </Button>
                </Box>
                {settled && (
                    <Box role="status" sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.75, mb: 1, color: 'text.secondary' }}>
                        <Info sx={{ fontSize: 14, mt: 0.25, flexShrink: 0 }} />
                        <Typography variant="caption" sx={{ color: 'inherit' }}>{t('thread.requests.settled')}</Typography>
                    </Box>
                )}
                {empty ? (
                    !settled && <Typography role="status" variant="body2" sx={{ color: 'text.secondary', py: 0.5 }}>{t('thread.requests.cleared')}</Typography>
                ) : (
                    <RequestCards thread={thread} requests={requests} onSettled={() => setSettled(true)} />
                )}
            </Box>
        </Popover>
    );
}
