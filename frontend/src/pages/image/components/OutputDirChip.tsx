import { Box, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { CopyIconButton } from '@/components/CopyIconButton';
import { FolderOpen } from '@/components/icons';
import { fontMono } from '@/theme/fonts';

// Splits a path into what may be cut and what may not: the last folder is
// the part that tells one output folder from another, so it always shows,
// and the leading part gives way first.
export const splitPathTail = (path: string): { head: string; tail: string } => {
    const trimmed = path.replace(/[\\/]+$/, '');
    const index = Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\'));
    if (index <= 0) return { head: '', tail: trimmed };
    return { head: trimmed.slice(0, index), tail: trimmed.slice(index) };
};

interface OutputDirChipProps {
    path: string;
}

// Where generated images land, as a one-line readout in the card's top-right
// corner: out of the way of the workbench below, and never taller than the
// title it sits beside. A long path is shortened from the front, the full one
// is on hover, and the copy button hands over the whole thing.
const OutputDirChip: React.FC<OutputDirChipProps> = ({ path }) => {
    const { t } = useTranslation();
    const label = t('playground.outputDirLabel', { defaultValue: 'Generated images are saved to' });
    const { head, tail } = splitPathTail(path);
    return (
        <Stack
            direction="row"
            spacing={0.5}
            sx={{
                alignItems: 'center',
                minWidth: 0,
                maxWidth: { xs: 200, sm: 360, lg: 480 },
                pl: 1,
                pr: 0.25,
                py: 0.25,
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1,
                color: 'text.secondary',
            }}
        >
            <Tooltip
                title={(
                    <>
                        {label}:
                        <Box component="span" sx={{ display: 'block', fontFamily: fontMono, wordBreak: 'break-all' }}>{path}</Box>
                    </>
                )}
            >
                <Stack
                    direction="row"
                    aria-label={`${label}: ${path}`}
                    sx={{ alignItems: 'center', gap: 0.75, minWidth: 0, cursor: 'default' }}
                >
                    <FolderOpen sx={{ fontSize: 16, flexShrink: 0 }} />
                    <Typography
                        component="span"
                        variant="caption"
                        sx={{ display: 'flex', minWidth: 0, fontFamily: fontMono, whiteSpace: 'nowrap' }}
                    >
                        {head && (
                            <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', minWidth: '1.5em' }}>
                                {head}
                            </Box>
                        )}
                        <Box component="span" sx={{ flexShrink: 0 }}>{tail}</Box>
                    </Typography>
                </Stack>
            </Tooltip>
            <CopyIconButton
                value={path}
                label={t('common.copy', { defaultValue: 'Copy' })}
                copiedLabel={t('common.copied', { defaultValue: 'Copied!' })}
                iconSize={14}
                sx={{ p: 0.25, flexShrink: 0 }}
            />
        </Stack>
    );
};

export default OutputDirChip;
