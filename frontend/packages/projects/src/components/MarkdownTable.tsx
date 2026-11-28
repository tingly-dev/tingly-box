import { memo, useRef, type ReactNode } from 'react';
import { Box, IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useCopyFeedback } from '@tingly/ui';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';
import { Check, ContentCopy, ErrorOutline } from './icons';
import { tableToMarkdown } from './tableToMarkdown';

/** A table in a reply, in its own card with a copy button in a header row.
 * The button sits above the table rather than inside it, so it stays put when
 * a wide table scrolls sideways. Copying yields a Markdown table rebuilt from
 * the rendered cells (see tableToMarkdown) — the source Markdown, not the
 * flattened visible text. */
export const TableBlock = memo(function TableBlock({ children }: { children?: ReactNode }) {
    const { t } = useTranslation('projects');
    const { copied, failed, copy } = useCopyFeedback();
    const table = useRef<HTMLTableElement>(null);
    const label = t('markdown.copyTable', { defaultValue: 'Copy table' });
    const done = t('common.copied');
    const error = t('phone.copyFailed', { defaultValue: 'Could not copy' });
    return (
        <Box sx={{ position: 'relative', my: 1.5, border: 1, borderColor: 'divider', borderRadius: radius.row, overflow: 'hidden' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', px: 1.5, py: 0.25, bgcolor: 'action.hover', fontSize: fontSizes.sm, color: 'text.secondary' }}>
                <span>{t('markdown.table', { defaultValue: 'Table' })}</span>
                <Tooltip title={copied ? done : failed ? error : label}>
                    <IconButton
                        size="small"
                        aria-label={copied ? done : failed ? error : label}
                        data-copy-state={copied ? 'copied' : failed ? 'failed' : 'idle'}
                        onClick={() => { if (table.current) void copy(tableToMarkdown(table.current)); }}
                        sx={{ color: copied ? 'success.main' : failed ? 'error.main' : 'text.secondary' }}
                    >
                        {copied ? <Check fontSize="inherit" color="success" /> : failed ? <ErrorOutline fontSize="inherit" color="error" /> : <ContentCopy fontSize="inherit" />}
                    </IconButton>
                </Tooltip>
            </Box>
            {/* The card owns the spacing. The inner box scrolls sideways as a
                whole and the table keeps its content width, so columns stay
                readable instead of collapsing to one letter per line; the
                inline styles override the shared `& table { display:block;
                overflow-x:auto }` that would otherwise scroll and squash the
                table itself. */}
            <Box sx={{ overflowX: 'auto' }}>
                <table
                    ref={table}
                    style={{ display: 'table', overflowX: 'visible', width: 'max-content', minWidth: '100%', borderCollapse: 'collapse', marginTop: 0, marginBottom: 0 }}
                >
                    {children}
                </table>
            </Box>
        </Box>
    );
});
