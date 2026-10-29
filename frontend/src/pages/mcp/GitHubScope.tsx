import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Button, Chip, Stack, TextField, Typography } from '@mui/material';
import type { MCPProfile } from './types';

const REPO = /^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\/[A-Za-z0-9._-]+$/;

// Repository scope of the GitHub integration for one MCP profile. Empty means
// every repository the token can reach; once set, calls outside the list are
// refused by the gateway.
export default function GitHubScope({
    profile,
    busy,
    onSave,
}: {
    profile: MCPProfile;
    busy: boolean;
    onSave: (next: MCPProfile) => void;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const repos = profile.github_repos || [];
    const [draft, setDraft] = useState('');
    const value = draft.trim().replace(/^https:\/\/github\.com\//, '');
    const invalid = value !== '' && !REPO.test(value);
    const add = () => {
        if (!value || invalid || repos.some((r) => r.toLowerCase() === value.toLowerCase())) return;
        onSave({ ...profile, github_repos: [...repos, value] });
        setDraft('');
    };
    return (
        <Box component="section" aria-label={label('githubScope', 'GitHub repositories')}>
            <Typography variant="h6" sx={{ fontWeight: 600, mb: 0.5 }}>
                {label('githubScope', 'GitHub repositories')}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                {repos.length === 0
                    ? label('githubScopeAll', 'Not limited: GitHub tools in this profile can reach every repository the token can. Add repositories to limit them.')
                    : label('githubScopeLimited', 'GitHub tools in this profile are refused for any repository not listed here.')}
            </Typography>
            <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 1, mb: 1 }}>
                {repos.map((repo) => (
                    <Chip
                        key={repo}
                        label={repo}
                        disabled={busy}
                        onDelete={() => onSave({ ...profile, github_repos: repos.filter((r) => r !== repo) })}
                    />
                ))}
            </Stack>
            <Stack direction="row" spacing={1}>
                <TextField
                    size="small"
                    value={draft}
                    placeholder="owner/repo"
                    error={invalid}
                    helperText={invalid ? label('githubRepoInvalid', 'Use the form owner/repo') : undefined}
                    onChange={(e) => setDraft(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && add()}
                />
                <Button variant="outlined" disabled={busy || !value || invalid} onClick={add}>
                    {label('githubRepoAdd', 'Add repository')}
                </Button>
            </Stack>
        </Box>
    );
}
