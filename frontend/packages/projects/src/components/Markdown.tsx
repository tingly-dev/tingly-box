import { memo, useId, useLayoutEffect, useRef, useState } from 'react';
import ReactMarkdown, { type Components, type Options } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Box, ButtonBase, IconButton, Tooltip, useTheme } from '@mui/material';
import { Highlight, themes } from 'prism-react-renderer';
import { useTranslation } from 'react-i18next';
import { Check, ContentCopy, ErrorOutline, ExpandMore } from './icons';
import { TableBlock } from './MarkdownTable';
import { useCopyFeedback } from '@tingly/ui';
import { usePhoneShell } from '@tingly/ui/hooks/usePhoneShell';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { conversationMarkdown } from '../transcript/conversationStyles';
import { remarkAutolinkBoundary } from '../transcript/autolink';
import { remarkBreaks } from '../transcript/breaks';

type RemarkPlugin = NonNullable<Options['remarkPlugins']>[number];

/** A fenced code block: highlighted, with a copy button. On a phone a long
 * block is height-capped, with its own 展开/收起 row inside the box; the copy
 * icon sits on the right of the header (short) or of that row (long), matching
 * the desktop header's icon. */
export function CodeBlock({ code, language }: { code: string; language: string }) {
    const theme = useTheme();
    const { t } = useTranslation('projects');
    const phone = usePhoneShell();
    const { copied, failed, copy } = useCopyFeedback();
    const pre = useRef<HTMLDivElement>(null);
    const bodyId = useId();
    const [overflows, setOverflows] = useState(false);
    const [open, setOpen] = useState(false);
    useLayoutEffect(() => {
        if (!phone) return;
        const el = pre.current;
        if (!el || open) return;
        const measure = () => setOverflows(el.scrollHeight > el.clientHeight + 1);
        measure();
        if (typeof ResizeObserver === 'undefined') return;
        const observer = new ResizeObserver(measure);
        observer.observe(el);
        return () => observer.disconnect();
    }, [phone, open, code]);
    // The cap itself is what makes the block measurable: apply it whenever the
    // phone shows a collapsed block, then the effect finds whether it overflows.
    const capping = phone && !open;
    const footer = phone && (overflows || open);
    const copyButton = (
        <Tooltip title={copied ? t('common.copied') : failed ? t('phone.copyFailed', { defaultValue: 'Could not copy' }) : t('common.copy')}>
            <IconButton
                size="small"
                onClick={() => copy(code)}
                aria-label={copied ? t('common.copied') : failed ? t('phone.copyFailed', { defaultValue: 'Could not copy' }) : t('common.copy')}
                data-copy-state={copied ? 'copied' : failed ? 'failed' : 'idle'}
                sx={{ color: copied ? 'success.main' : failed ? 'error.main' : 'text.secondary' }}
            >
                {copied ? <Check fontSize="inherit" color="success" /> : failed ? <ErrorOutline fontSize="inherit" color="error" /> : <ContentCopy fontSize="inherit" />}
            </IconButton>
        </Tooltip>
    );
    return (
        <Box sx={{ position: 'relative', my: 1.5, borderRadius: radius.row, border: 1, borderColor: 'divider', overflow: 'hidden' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', px: 1.5, py: 0.25, bgcolor: 'action.hover', fontSize: fontSizes.sm, color: 'text.secondary' }}>
                <span>{language || 'text'}</span>
                {(!phone || !footer) && copyButton}
            </Box>
            <Highlight code={code} language={language || 'text'} theme={theme.palette.mode === 'dark' ? themes.vsDark : themes.github}>
                {({ style, tokens, getLineProps, getTokenProps }) => (
                    <Box ref={pre} id={bodyId} component="pre" sx={{
                        m: 0, p: 1.5, overflowX: 'auto', fontSize: fontSizes.base, lineHeight: 1.55, whiteSpace: 'pre', wordBreak: 'normal',
                        ...(capping && {
                            maxHeight: 210,
                            overflowY: 'hidden',
                            WebkitMaskImage: (t: typeof theme) => `linear-gradient(to bottom, ${t.palette.common.black} calc(100% - 26px), transparent)`,
                            maskImage: (t: typeof theme) => `linear-gradient(to bottom, ${t.palette.common.black} calc(100% - 26px), transparent)`,
                        }),
                    }} style={{ ...style, background: 'transparent', whiteSpace: 'pre', wordBreak: 'normal' }}>
                        {tokens.map((line, i) => (
                            <div key={i} {...getLineProps({ line })}>
                                {line.map((token, key) => <span key={key} {...getTokenProps({ token })} />)}
                            </div>
                        ))}
                    </Box>
                )}
            </Highlight>
            {footer && (
                <Box sx={{ display: 'flex', alignItems: 'center', minHeight: 44, px: 1.5, borderTop: 1, borderColor: 'divider', bgcolor: 'background.paper' }}>
                    <ButtonBase onClick={() => setOpen((value) => !value)} aria-expanded={open} aria-controls={bodyId} sx={{ minHeight: 44, gap: 0.75, color: 'primary.main', fontSize: fontSizes.sm, fontWeight: 600 }}>
                        {open
                            ? t('phone.collapseCode', { defaultValue: 'Collapse code' })
                            : t('phone.expandCode', { defaultValue: 'Show full code' })}
                        <ExpandMore sx={{ fontSize: 16, transform: open ? 'rotate(180deg)' : 'none' }} />
                    </ButtonBase>
                    <Box sx={{ flex: 1 }} />
                    {copyButton}
                </Box>
            )}
        </Box>
    );
}

// Targets a `safe` document may link to; everything else (javascript:, data:,
// file:, relative paths, anchors) loses its target and is shown as plain text.
const SAFE_URL = /^(https?|mailto):/i;
const safeUrl = (url: string) => (SAFE_URL.test(url) ? url : '');

const safeComponents: Components = {
    a: ({ node: _node, href, children }) => href
        ? <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>
        : <span>{children}</span>,
    // A remote image is a request the document's author chooses, so it is never made: the alt text stands in.
    img: ({ node: _node, alt, src }) => (
        <Box component="span" title={typeof src === 'string' && /^https?:/i.test(src) ? src : undefined} sx={{ color: 'text.secondary', fontStyle: 'italic' }}>
            {alt || '[image]'}
        </Box>
    ),
};

const markdownComponents: Components = {
    a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noreferrer" />,
    pre: ({ children }) => <>{children}</>,
    table: ({ children }) => <TableBlock>{children}</TableBlock>,
    code: ({ className, children }) => {
        const match = /language-(\w+)/.exec(className ?? '');
        const code = String(children ?? '');
        if (!match && !code.includes('\n')) {
            return <code>{children}</code>;
        }
        return <CodeBlock code={code.replace(/\n$/, '')} language={match?.[1] ?? ''} />;
    },
};
const safeMarkdownComponents: Components = { ...markdownComponents, ...safeComponents };

/**
 * Renders assistant Markdown (GFM) with highlighted, copyable code blocks.
 * `safe` is for documents nobody here wrote (a file in the library): links
 * are limited to http, https and mailto and open without opener or referrer,
 * and images are never loaded.
 */
export const Markdown = memo(function Markdown({ text, conversation = false, safe = false }: { text: string; conversation?: boolean; safe?: boolean }) {
    return (
        <Box
            sx={conversation ? {
                ...conversationMarkdown,
                '& > :last-child': { mb: 0 },
                '& :not(pre) > code': { px: 0.5, py: 0.125, borderRadius: radius.chip, bgcolor: 'action.hover', fontSize: fontSizes.inlineCode, fontFamily: fontMono },
                '& pre, & pre code': { fontFamily: fontMono },
            } : {
                wordBreak: 'break-word',
                color: 'text.primary',
                '& p': { my: 1 },
                '& p:first-of-type': { mt: 0 },
                '& ul, & ol': { pl: 3, my: 1 },
                '& li': { my: 0.25 },
                '& h1, & h2, & h3, & h4': { mt: 2, mb: 1, lineHeight: 1.3 },
                // Detail Markdown keeps its own heading sizes, deliberately larger than
        // conversation replies (ConversationPane.test pins both).
        '& h1': { fontSize: '1.4rem' },
                '& h2': { fontSize: '1.2rem' },
                '& h3': { fontSize: '1.05rem' },
                '& a': { color: 'primary.main' },
                '& blockquote': { m: 0, my: 1, pl: 2, borderLeft: 3, borderColor: 'divider', color: 'text.secondary' },
                '& :not(pre) > code': { px: 0.5, py: 0.1, borderRadius: radius.chip, bgcolor: 'action.hover', fontSize: fontSizes.inlineCode, fontFamily: fontMono },
                '& pre, & pre code': { fontFamily: fontMono },
                '& table': { borderCollapse: 'collapse', my: 1.5, display: 'block', overflowX: 'auto' },
                '& th, & td': { border: 1, borderColor: 'divider', px: 1.25, py: 0.5, textAlign: 'left' },
                '& th': { bgcolor: 'action.hover' },
                '& hr': { border: 0, borderTop: 1, borderColor: 'divider', my: 2 },
            }}
        >
            <ReactMarkdown
                remarkPlugins={[remarkGfm, remarkBreaks, remarkAutolinkBoundary as RemarkPlugin]}
                urlTransform={safe ? safeUrl : undefined}
                components={safe ? safeMarkdownComponents : markdownComponents}
            >
                {text}
            </ReactMarkdown>
        </Box>
    );
});
