import { useState } from 'react';
import { Box, Button, Chip, IconButton, MenuItem, Select, TextField, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap } from '../../client';
import { Branch, Close, Warning } from '../icons';
import { DirectoryPicker } from '../common/DirectoryPicker';
import { fontMono, fontSizes } from '@tingly/ui/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

export interface RepoEntry {
    root: string;
    branch: string;
    dirty?: boolean;
    kind: 'git' | 'directory';
    workspace_mode: 'shared' | 'worktree';
}

// Temporary protocol types until Projects OpenAPI is regenerated.
export type DirectoryConfig = { path: string; workspace_mode: 'shared' | 'worktree' };
export type DirectoryProject = { primary_repo_path?: string; repos: { path: string; base_branch: string; kind?: 'git' | 'directory'; workspace_mode?: 'shared' | 'worktree' }[]; disable_worktree?: boolean };
export function projectDirectories(project: DirectoryProject): RepoEntry[] {
    return project.repos.map((r) => ({ root: r.path, branch: r.base_branch, kind: r.kind ?? 'git', workspace_mode: r.kind === 'directory' ? 'shared' : r.workspace_mode ?? (project.disable_worktree ? 'shared' : 'worktree') }));
}
export function directoryPayload(entries: RepoEntry[], primary: string) {
    return { directories: entries.map((r): DirectoryConfig => ({ path: r.root, workspace_mode: r.kind === 'directory' ? 'shared' : r.workspace_mode })), primary_repo_path: primary };
}
export function primaryPath(project: DirectoryProject) { return project.primary_repo_path ?? project.repos[0]?.path ?? ''; }

const mono = { fontFamily: fontMono } as const;

/**
 * Adds local directories by path. Each path is inspected and shown as the
 * root the threads will use. Picking a directory adds it immediately; a typed
 * path keeps the Add button.
 */
export function RepoField({ value, onChange, primary = value[0]?.root ?? '', onPrimaryChange }: { value: RepoEntry[]; onChange: (v: RepoEntry[]) => void; primary?: string; onPrimaryChange?: (p: string) => void }) {
    const { t } = useTranslation('projects');
    const [path, setPath] = useState('');
    const [error, setError] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);
    const [browsing, setBrowsing] = useState(false);

    const add = async (p: string) => {
        const target = p.trim();
        if (!target) return;
        setBusy(true);
        setError(null);
        try {
            const post = api.POST as unknown as (url: string, options: { body: { path: string } }) => Promise<{ data?: { ok: boolean; root?: string; kind: 'git' | 'directory'; branch?: string; dirty?: boolean; error?: string }; error?: unknown; response: Response }>;
            const r = await unwrap(post('/api/v1/projects/directories/inspect', { body: { path: target } }));
            if (!r.ok || !r.root) {
                setError(r.error ?? t('repo.invalid'));
                setPath(target);
                return;
            }
            if (!value.some((v) => v.root === r.root)) {
                onChange([...value, { root: r.root, branch: r.branch ?? '', kind: r.kind, workspace_mode: r.kind === 'directory' ? 'shared' : 'worktree', dirty: r.dirty }]);
                if (!value.length) onPrimaryChange?.(r.root);
            }
            setPath('');
        } catch (e) {
            setError((e as Error).message);
            setPath(target);
        } finally {
            setBusy(false);
        }
    };

    return (
        <Box>
            {value.map((r, i) => (
                <Box key={r.root} sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 1, py: 0.75, px: 1.25, mb: 0.75, border: 1, borderColor: 'divider', borderRadius: radius.row }}>
                    <Typography sx={{ ...mono, fontSize: fontSizes.base, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: 'text.primary' }} title={r.root}>
                        {r.root}
                    </Typography>
                    {r.kind === 'git' && <Tooltip title={t('repo.baseHint')}>
                        <Chip size="small" variant="outlined" icon={<Branch sx={{ fontSize: 14 }} />} label={r.branch} sx={{ ...mono, fontSize: fontSizes.sm }} />
                    </Tooltip>}
                    {r.dirty && (
                        <Tooltip title={t('repo.dirtyHint')}>
                            <Warning sx={{ fontSize: 16 }} color="warning" />
                        </Tooltip>
                    )}
                    <Select size="small" value={r.workspace_mode} disabled={r.kind !== 'git'} inputProps={{ 'aria-label': `${t('repo.mode')} ${r.root}` }} onChange={(e) => onChange(value.map((v, j) => j === i ? { ...v, workspace_mode: e.target.value as RepoEntry['workspace_mode'] } : v))}>
                        <MenuItem value="shared">{t('threadSettings.workspace.shared')}</MenuItem>
                        {r.kind === 'git' && <MenuItem value="worktree">{t('threadSettings.workspace.worktree')}</MenuItem>}
                    </Select>
                    {onPrimaryChange && <Button size="small" onClick={() => onPrimaryChange(r.root)} disabled={primary === r.root}>{t(primary === r.root ? 'repo.primary' : 'repo.makePrimary')}</Button>}
                    <IconButton size="small" onClick={() => { const remaining = value.filter((_, j) => j !== i); onChange(remaining); if (primary === r.root) onPrimaryChange?.(remaining[0]?.root ?? ''); }} aria-label={t('common.delete')}>
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
                            void add(path);
                        }
                    }}
                    placeholder={t('repo.placeholder')}
                    error={!!error}
                    helperText={error ?? undefined}
                    slotProps={{ htmlInput: { style: { ...mono, fontSize: fontSizes.base }, spellCheck: false } }}
                />
                <Button variant="outlined" onClick={() => setBrowsing(true)} sx={{ flexShrink: 0, height: 40 }}>{t('directory.browse')}</Button>
                <Button variant="outlined" onClick={() => void add(path)} disabled={!path.trim() || busy} sx={{ flexShrink: 0, height: 40 }}>
                    {t('repo.add')}
                </Button>
            </Box>
            {/* Picking a folder is the decision to add it, as in AddWorkdirDialog. */}
            {browsing && <DirectoryPicker onClose={() => setBrowsing(false)} onSelect={(selected) => { setError(null); setBrowsing(false); void add(selected); }} />}
        </Box>
    );
}
