import {Alert, Box, Button, Link, MenuItem, Stack, TextField, Typography} from '@mui/material';
import React, {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    clearGitHubToken,
    disconnectGitHub,
    getGitHubStatus,
    setGitHubToken,
    startGitHubConnect,
    submitManifest,
    syncGitHub,
    type GitHubStatus,
} from '@/services/githubApi';

/**
 * Connect GitHub (see .design/github.md): creates this tingly-box's own
 * GitHub App through GitHub's manifest flow, and shows what it is installed
 * on. A fine-grained token is the fallback for people who cannot create an App.
 */
const GitHubConnect: React.FC = () => {
    const {t} = useTranslation();
    const [status, setStatus] = useState<GitHubStatus>();
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const [ownerType, setOwnerType] = useState<'user' | 'org'>('user');
    const [org, setOrg] = useState('');
    const [showToken, setShowToken] = useState(false);
    const [token, setToken] = useState('');

    const load = useCallback(async () => {
        try {
            setStatus(await getGitHubStatus(true));
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        }
    }, []);

    useEffect(() => {
        void load();
    }, [load]);

    const run = async (fn: () => Promise<void>) => {
        setError('');
        setBusy(true);
        try {
            await fn();
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    const connect = () => run(async () => submitManifest(await startGitHubConnect(ownerType, org.trim())));

    if (!status) {
        return error ? <Alert severity="error" sx={{mt: 1}}>{error}</Alert> : null;
    }

    return (
        <Box sx={{mt: 1, p: 2, border: 1, borderColor: 'divider', borderRadius: 1}}>
            <Stack spacing={1.5}>
                {error && <Alert severity="error">{error}</Alert>}

                {status.connected ? (
                    <>
                        <Typography variant="body2">
                            {t('system.githubConnect.connectedAs', {
                                defaultValue: 'GitHub App {{slug}} ({{owner}})',
                                slug: status.app_slug,
                                owner: status.app_owner,
                            })}
                            {status.app_url && (
                                <>{' · '}<Link href={status.app_url} target="_blank" rel="noreferrer">github.com</Link></>
                            )}
                        </Typography>
                        {status.installations.length === 0 ? (
                            <Alert severity="warning">
                                {t('system.githubConnect.notInstalled', {
                                    defaultValue: 'The App is not installed on any account yet. Install it and pick the repositories agents may push to.',
                                })}
                            </Alert>
                        ) : (
                            status.installations.map((i) => (
                                <Typography key={i.id} variant="body2" color="text.secondary">
                                    {i.account}: {i.repository_selection === 'all'
                                        ? t('system.githubConnect.allRepos', {defaultValue: 'all repositories'})
                                        : t('system.githubConnect.repoCount', {
                                            defaultValue: '{{count}} repositories',
                                            count: i.repositories >= 0 ? i.repositories : 0,
                                        })}
                                </Typography>
                            ))
                        )}
                        <Stack direction="row" spacing={1}>
                            {status.install_url && (
                                <Button size="small" href={status.install_url} target="_blank" rel="noreferrer">
                                    {t('system.githubConnect.addRepos', {defaultValue: 'Add repositories'})}
                                </Button>
                            )}
                            <Button size="small" disabled={busy} onClick={() => run(async () => { await syncGitHub(); await load(); })}>
                                {t('system.githubConnect.refresh', {defaultValue: 'Refresh'})}
                            </Button>
                            <Button size="small" color="error" disabled={busy} onClick={() => run(async () => { await disconnectGitHub(); await load(); })}>
                                {t('system.githubConnect.disconnect', {defaultValue: 'Disconnect'})}
                            </Button>
                        </Stack>
                    </>
                ) : (
                    <>
                        <Typography variant="body2" color="text.secondary">
                            {t('system.githubConnect.intro', {
                                defaultValue: 'Creates a GitHub App that only this tingly-box holds the key to. Agents get read access; pushes and pull requests go through tingly-box and show up as the App.',
                            })}
                        </Typography>
                        <Stack direction="row" spacing={1} sx={{alignItems: "center"}}>
                            <TextField select size="small" value={ownerType} sx={{minWidth: 160}}
                                       onChange={(e) => setOwnerType(e.target.value as 'user' | 'org')}>
                                <MenuItem value="user">{t('system.githubConnect.personal', {defaultValue: 'My account'})}</MenuItem>
                                <MenuItem value="org">{t('system.githubConnect.organization', {defaultValue: 'An organization'})}</MenuItem>
                            </TextField>
                            {ownerType === 'org' && (
                                <TextField size="small" value={org} onChange={(e) => setOrg(e.target.value)}
                                           placeholder={t('system.githubConnect.orgName', {defaultValue: 'Organization name'})}/>
                            )}
                            <Button variant="contained" size="small" disabled={busy || (ownerType === 'org' && !org.trim())} onClick={connect}>
                                {t('system.githubConnect.connect', {defaultValue: 'Connect GitHub'})}
                            </Button>
                        </Stack>
                    </>
                )}

                {status.has_pat ? (
                    <Stack direction="row" spacing={1} sx={{alignItems: "center"}}>
                        <Typography variant="body2" color="text.secondary">
                            {t('system.githubConnect.patSaved', {defaultValue: 'A personal access token is saved as the fallback.'})}
                        </Typography>
                        <Button size="small" color="error" disabled={busy} onClick={() => run(async () => { await clearGitHubToken(); await load(); })}>
                            {t('system.githubConnect.removeToken', {defaultValue: 'Remove'})}
                        </Button>
                    </Stack>
                ) : !status.connected && (
                    showToken ? (
                        <Stack direction="row" spacing={1}>
                            <TextField size="small" type="password" value={token} onChange={(e) => setToken(e.target.value)}
                                       placeholder="github_pat_…" sx={{flex: 1}}/>
                            <Button size="small" disabled={busy || !token.trim()}
                                    onClick={() => run(async () => { await setGitHubToken(token.trim()); setToken(''); setShowToken(false); await load(); })}>
                                {t('system.githubConnect.saveToken', {defaultValue: 'Save'})}
                            </Button>
                        </Stack>
                    ) : (
                        <Link component="button" variant="body2" underline="hover" sx={{alignSelf: 'flex-start'}} onClick={() => setShowToken(true)}>
                            {t('system.githubConnect.useToken', {defaultValue: "Can't create an App? Use a personal access token"})}
                        </Link>
                    )
                )}
            </Stack>
        </Box>
    );
};

export default GitHubConnect;
