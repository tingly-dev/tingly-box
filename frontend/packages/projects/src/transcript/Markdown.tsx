import XMarkdown, {type ComponentProps} from '@ant-design/x-markdown';
import {Box} from '@mui/material';
import {fontMono} from '@tingly/ui/theme/fonts';
import type {ReactNode} from 'react';
import {Children, isValidElement} from 'react';
import {useTranscriptHost} from './context';
import { radius } from '@tingly/ui/theme/radius';
import { conversationMarkdown } from './conversationStyles';

// Plain text of a rendered node tree: a fenced block's code reaches the code
// component as parsed children, but a code block takes a string.
const textOf = (node: ReactNode): string => {
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join('');
    if (isValidElement<{children?: ReactNode}>(node)) return textOf(node.props.children);
    return '';
};

// PlainCode is a fenced block when the host passes no code block of its own.
const PlainCode = ({code}: {code: string; language: string}) => (
    <Box
        component="pre"
        sx={{fontFamily: fontMono, fontSize: '0.85em', m: 0, my: 1.5, p: 1.5, borderRadius: radius.row, bgcolor: 'action.hover', overflow: 'auto', maxHeight: 480}}
    >
        {code}
    </Box>
);

const Code = ({block, lang, children}: ComponentProps) => {
    const {codeBlock: CodeBlock = PlainCode} = useTranscriptHost();
    if (block) {
        const code = Children.toArray(children).map(textOf).join('').replace(/\n$/, '');
        return <CodeBlock code={code} language={lang?.split(/\s/)[0] ?? ''}/>;
    }
    return (
        <Box
            component="code"
            sx={{fontFamily: fontMono, fontSize: '0.85em', px: 0.5, py: 0.125, borderRadius: radius.chip, bgcolor: 'action.hover'}}
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
        sx={{...conversationMarkdown, '& > .x-markdown > :last-child': {mb: 0}}}
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
