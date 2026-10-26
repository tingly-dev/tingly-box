import { memo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Box, IconButton, Tooltip, useTheme } from '@mui/material';
import { Highlight, themes } from 'prism-react-renderer';
import { useTranslation } from 'react-i18next';
import { Check, ContentCopy } from './icons';
import { useCopyFeedback } from '../hooks/useCopyFeedback';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { conversationMarkdown } from '../transcript/conversationStyles';

/** A fenced code block: highlighted, with a copy button. */
export function CodeBlock({ code, language }: { code: string; language: string }) {
    const theme = useTheme();
    const { t } = useTranslation('projects');
    const { copied, copy } = useCopyFeedback();
    return (
        <Box sx={{ position: 'relative', my: 1.5, borderRadius: radius.row, border: 1, borderColor: 'divider', overflow: 'hidden' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', px: 1.5, py: 0.25, bgcolor: 'action.hover', fontSize: fontSizes.sm, color: 'text.secondary' }}>
                <span>{language || 'text'}</span>
                <Tooltip title={copied ? t('common.copied') : t('common.copy')}>
                    <IconButton size="small" onClick={() => copy(code)} aria-label={t('common.copy')}>
                        {copied ? <Check fontSize="inherit" color="success" /> : <ContentCopy fontSize="inherit" />}
                    </IconButton>
                </Tooltip>
            </Box>
            <Highlight code={code} language={language || 'text'} theme={theme.palette.mode === 'dark' ? themes.vsDark : themes.github}>
                {({ style, tokens, getLineProps, getTokenProps }) => (
                    <Box component="pre" sx={{ m: 0, p: 1.5, overflowX: 'auto', fontSize: fontSizes.base, lineHeight: 1.55 }} style={{ ...style, background: 'transparent' }}>
                        {tokens.map((line, i) => (
                            <div key={i} {...getLineProps({ line })}>
                                {line.map((token, key) => <span key={key} {...getTokenProps({ token })} />)}
                            </div>
                        ))}
                    </Box>
                )}
            </Highlight>
        </Box>
    );
}

/** Renders assistant Markdown (GFM) with highlighted, copyable code blocks. */
export const Markdown = memo(function Markdown({ text, conversation = false }: { text: string; conversation?: boolean }) {
    return (
        <Box
            sx={conversation ? {
                ...conversationMarkdown,
                '& > :last-child': { mb: 0 },
                '& :not(pre) > code': { px: 0.5, py: 0.125, borderRadius: radius.chip, bgcolor: 'action.hover', fontSize: '0.85em', fontFamily: fontMono },
                '& pre, & pre code': { fontFamily: fontMono },
            } : {
                wordBreak: 'break-word',
                color: 'text.primary',
                '& p': { my: 1 },
                '& p:first-of-type': { mt: 0 },
                '& ul, & ol': { pl: 3, my: 1 },
                '& li': { my: 0.25 },
                '& h1, & h2, & h3, & h4': { mt: 2, mb: 1, lineHeight: 1.3 },
                '& h1': { fontSize: '1.4rem' },
                '& h2': { fontSize: '1.2rem' },
                '& h3': { fontSize: '1.05rem' },
                '& a': { color: 'primary.main' },
                '& blockquote': { m: 0, my: 1, pl: 2, borderLeft: 3, borderColor: 'divider', color: 'text.secondary' },
                '& :not(pre) > code': { px: 0.5, py: 0.1, borderRadius: radius.chip, bgcolor: 'action.hover', fontSize: '0.85em', fontFamily: fontMono },
                '& pre, & pre code': { fontFamily: fontMono },
                '& table': { borderCollapse: 'collapse', my: 1.5, display: 'block', overflowX: 'auto' },
                '& th, & td': { border: 1, borderColor: 'divider', px: 1.25, py: 0.5, textAlign: 'left' },
                '& th': { bgcolor: 'action.hover' },
                '& hr': { border: 0, borderTop: 1, borderColor: 'divider', my: 2 },
            }}
        >
            <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                components={{
                    a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noreferrer" />,
                    pre: ({ children }) => <>{children}</>,
                    code: ({ className, children }) => {
                        const match = /language-(\w+)/.exec(className || '');
                        const code = String(children ?? '');
                        if (!match && !code.includes('\n')) {
                            return <code>{children}</code>;
                        }
                        return <CodeBlock code={code.replace(/\n$/, '')} language={match?.[1] ?? ''} />;
                    },
                }}
            >
                {text}
            </ReactMarkdown>
        </Box>
    );
});
