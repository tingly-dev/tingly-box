import { useState } from 'react';
import { Box, Button, Chip, IconButton, TextField, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap } from '../../client';
import { Branch, Close, Warning } from '../icons';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';

export interface RepoEntry {
    root: string;
    branch: string;
    dirty?: boolean;
}

const mono = { fontFamily: fontMono } as const;

/**
 * Adds local repositories by path. Each path is checked right away and shown
 * as what threads will actually use: the repository root and the branch they
 * branch from.
 */
export function RepoField({ value, onChange }: { value: RepoEntry[]; onChange: (v: RepoEntry[]) => void }) {
    const { t } = useTranslation('projects');
    const [path, setPath] = useState('');
    const [error, setError] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);

    const add = async () => {
        const p = path.trim();
        if (!p) return;
        setBusy(true);
        setError(null);
        try {
            const r = await unwrap(api.POST('/api/v1/projects/repos/inspect', { body: { path: p } }));
            if (!r.ok || !r.root) {
                setError(r.error ?? t('repo.invalid'));
                return;
            }
            if (!value.some((v) => v.root === r.root)) {
                onChange([...value, { root: r.root, branch: r.branch ?? '', dirty: r.dirty }]);
            }
            setPath('');
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <Box>
            {value.map((r, i) => (
                <Box key={r.root} sx={{ display: 'flex', alignItems: 'center', gap: 1, py: 0.75, px: 1.25, mb: 0.75, border: 1, borderColor: 'divider', borderRadius: 2 }}>
                    <Typography sx={{ ...mono, fontSize: fontSizes.lg, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: 'text.primary' }} title={r.root}>
                        {r.root}
                    </Typography>
                    <Tooltip title={t('repo.baseHint')}>
                        <Chip size="small" variant="outlined" icon={<Branch sx={{ fontSize: 14 }} />} label={r.branch} sx={{ ...mono, fontSize: fontSizes.sm }} />
                    </Tooltip>
                    {r.dirty && (
                        <Tooltip title={t('repo.dirtyHint')}>
                            <Warning sx={{ fontSize: 16 }} color="warning" />
                        </Tooltip>
                    )}
                    <IconButton size="small" onClick={() => onChange(value.filter((_, j) => j !== i))} aria-label={t('common.delete')}>
                        <Close fontSize="small" />
                    </IconButton>
                </Box>
            ))}
            <Box sx={{ display: 'flex', gap: 1 }}>
                <TextField
                    size="small"
                    fullWidth
                    value={path}
                    onChange={(e) => setPath(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                            e.preventDefault();
                            void add();
                        }
                    }}
                    placeholder={t('repo.placeholder')}
                    error={!!error}
                    helperText={error ?? undefined}
                    slotProps={{ htmlInput: { style: { ...mono, fontSize: fontSizes.lg }, spellCheck: false } }}
                />
                <Button variant="outlined" onClick={() => void add()} disabled={!path.trim() || busy} sx={{ flexShrink: 0, height: 40 }}>
                    {t('repo.add')}
                </Button>
            </Box>
        </Box>
    );
}
