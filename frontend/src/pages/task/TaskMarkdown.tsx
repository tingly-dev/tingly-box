import XMarkdown from '@ant-design/x-markdown';
import { Box } from '@mui/material';

export default function TaskMarkdown({ children, compact = false }: { children: string; compact?: boolean }) {
  return <Box sx={{
    minWidth: 0,
    overflowX: 'auto',
    overflowWrap: 'anywhere',
    fontSize: compact ? '0.8125rem' : '0.875rem',
    lineHeight: 1.65,
    color: 'inherit',
    '& .ant-md, & .ant-markdown': {
      bgcolor: 'transparent',
      color: 'inherit',
      fontSize: 'inherit',
      lineHeight: 'inherit',
    },
    '& .ant-markdown > :first-of-type': { mt: 0 },
    '& .ant-markdown > :last-of-type': { mb: 0 },
    '& p': { my: 0.75 },
    '& h1, & h2, & h3, & h4, & h5, & h6': {
      mt: 1.5,
      mb: 0.75,
      color: 'text.primary',
      lineHeight: 1.3,
    },
    '& h1': { fontSize: '1.25rem' },
    '& h2': { fontSize: '1.125rem' },
    '& h3, & h4, & h5, & h6': { fontSize: '1rem' },
    '& ul, & ol': { my: 0.75, pl: 2.75 },
    '& li': { my: 0.25 },
    '& blockquote': {
      my: 1,
      mx: 0,
      pl: 1.5,
      borderLeft: '3px solid',
      borderColor: 'divider',
      color: 'text.secondary',
    },
    '& a': { color: 'primary.main', textUnderlineOffset: '2px' },
    '& :not(pre) > code': {
      px: 0.5,
      py: 0.125,
      borderRadius: 0.75,
      bgcolor: 'action.hover',
      fontFamily: 'monospace',
      fontSize: '0.9em',
    },
    '& pre': {
      my: 1,
      p: 1.25,
      maxWidth: '100%',
      overflow: 'auto',
      border: '1px solid',
      borderColor: 'divider',
      borderRadius: 1.5,
      bgcolor: 'action.hover',
      fontSize: '0.8125rem',
      lineHeight: 1.55,
    },
    '& pre code': { p: 0, bgcolor: 'transparent', fontFamily: 'monospace' },
    '& table': { my: 1, width: 'max-content', minWidth: '100%', borderCollapse: 'collapse' },
    '& th, & td': { px: 1, py: 0.625, border: '1px solid', borderColor: 'divider', textAlign: 'left' },
    '& th': { bgcolor: 'action.hover', fontWeight: 600 },
    '& hr': { my: 1.5, border: 0, borderTop: '1px solid', borderColor: 'divider' },
  }}>
    <XMarkdown escapeRawHtml openLinksInNewTab>{children}</XMarkdown>
  </Box>;
}
