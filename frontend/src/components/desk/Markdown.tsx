import CodeBlock from '@/components/CodeBlock';
import XMarkdown, {type ComponentProps} from '@ant-design/x-markdown';
import {Box} from '@mui/material';
import type {ReactNode} from 'react';
import {Children, isValidElement} from 'react';

// Plain text of a rendered node tree: a fenced block's code reaches the code
// component as parsed children, but CodeBlock highlights a string.
const textOf = (node: ReactNode): string => {
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join('');
    if (isValidElement<{children?: ReactNode}>(node)) return textOf(node.props.children);
    return '';
};

const Code = ({block, lang, children}: ComponentProps) => {
    if (block) {
        const code = Children.toArray(children).map(textOf).join('').replace(/\n$/, '');
        return <CodeBlock code={code} language={lang?.split(/\s/)[0] ?? ''} maxHeight={480} sx={{margin: '12px 0'}}/>;
    }
    return (
        <Box
            component="code"
            sx={{fontFamily: 'monospace', fontSize: '0.85em', px: 0.5, py: 0.125, borderRadius: 0.5, bgcolor: 'action.hover'}}
        >
            {children}
        </Box>
    );
};

// CodeBlock draws its own container, so the <pre> around a fenced block
// would only add a second frame.
const Pre = ({children}: ComponentProps) => <>{children}</>;

const components = {code: Code, pre: Pre};

// Markdown renders an agent's reply. Raw HTML in the output is shown as text,
// never rendered: the reply comes from a model reading arbitrary files.
const Markdown = ({content}: {content: string}) => (
    <Box
        sx={{
            color: 'text.primary',
            fontSize: '0.9375rem',
            lineHeight: 1.7,
            wordBreak: 'break-word',
            '& p': {m: 0, mb: 1.25},
            '& > .x-markdown > :last-child, & p:last-child': {mb: 0},
            '& h1, & h2, & h3, & h4': {mt: 2, mb: 1, lineHeight: 1.35, fontWeight: 650, color: 'text.primary'},
            '& h1': {fontSize: '1.25rem'},
            '& h2': {fontSize: '1.125rem'},
            '& h3, & h4': {fontSize: '1rem'},
            '& ul, & ol': {m: 0, mb: 1.25, pl: 3},
            '& li': {my: 0.25},
            '& a': {color: 'primary.main'},
            '& blockquote': {m: 0, mb: 1.25, pl: 1.5, borderLeft: 3, borderColor: 'divider', color: 'text.secondary'},
            '& hr': {border: 0, borderTop: 1, borderColor: 'divider', my: 2},
            '& table': {borderCollapse: 'collapse', mb: 1.25, fontSize: '0.875rem', display: 'block', overflowX: 'auto'},
            '& th, & td': {border: 1, borderColor: 'divider', px: 1.25, py: 0.5, textAlign: 'left'},
            '& th': {bgcolor: 'action.hover', fontWeight: 600},
        }}
    >
        <XMarkdown
            content={content}
            components={components}
            escapeRawHtml
            openLinksInNewTab
            disableDefaultStyles
        />
    </Box>
);

export default Markdown;
