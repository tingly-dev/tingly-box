import React, { useMemo, useState } from 'react';
import { Alert, Box, Button, IconButton, LinearProgress, Stack, TextField, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from '@mui/material';
import { Close as RemoveIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import type { ProbeCurlResult } from '@/components/probe/runProbe';
import { CopyBlock } from '@/components/probe/ResultSections';
import { CopyIconButton } from '@/components/CopyIconButton';
import type { ProbeRequest } from '@/types/probe';

// PayloadPanel: "what actually goes out" — the request as POST
// /api/v2/probe/curl renders it, from the same builders the run uses. The
// body is editable: a hand edit becomes an override (top-level key → value,
// null = delete) that the backend applies after every builder and flag, to
// the run and to this rendering alike, so the panel can never disagree with
// the request that leaves the process (.design/bench.md §7).

const prettyJson = (raw?: string): string => {
    if (!raw) return '';
    try {
        return JSON.stringify(JSON.parse(raw), null, 2);
    } catch {
        return raw;
    }
};

export const PayloadPanel: React.FC<{
    request: ProbeRequest | null;
    curl: ProbeCurlResult | null;
    loading: boolean;
    direct: boolean;
    /** The request is a hand-written raw request (the body is the user's own). */
    rawMode: boolean;
    /** Why no request could be built locally (e.g. the raw body does not parse). */
    buildError?: string;
    headers: Record<string, string>;
    onHeadersChange: (next: Record<string, string>) => void;
    /** "Edit": hand the rendered body to the request editor as a raw request. */
    onEditBody: (body: string) => void;
}> = ({ request, curl, loading, direct, rawMode, buildError, headers, onHeadersChange, onEditBody }) => {
    const { t } = useTranslation();
    const [tab, setTab] = useState<'request' | 'curl'>('request');
    const [newHeader, setNewHeader] = useState<{ name: string; value: string } | null>(null);

    const data = curl?.success ? curl.data : undefined;
    const pretty = useMemo(() => prettyJson(data?.body), [data?.body]);
    const headerRows = useMemo(() => Object.entries(data?.headers ?? {}).sort(([a], [b]) => a.localeCompare(b)), [data?.headers]);
    const overriddenHeader = (name: string) => Object.keys(headers).some((h) => h.toLowerCase() === name.toLowerCase());
    const removedHeaders = Object.entries(headers).filter(([, v]) => v === '');

    const setHeader = (name: string, value: string) => onHeadersChange({ ...headers, [name]: value });
    const dropHeaderOverride = (name: string) => {
        const next = { ...headers };
        Object.keys(next).forEach((h) => { if (h.toLowerCase() === name.toLowerCase()) delete next[h]; });
        onHeadersChange(next);
    };

    if (!request) {
        return buildError ? (
            <Alert severity="error" variant="outlined" sx={{ py: 0.25, fontSize: '0.78rem' }}>
                <strong>{t('bench.buildFailed', { defaultValue: 'Could not build the request' })}</strong> — {buildError}
            </Alert>
        ) : (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {t('bench.noTarget', { defaultValue: 'Pick a target to see the payload.' })}
            </Typography>
        );
    }

    return (
        <Stack spacing={1.5}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <ToggleButtonGroup size="small" exclusive value={tab} onChange={(_, v) => v && setTab(v)}>
                    <ToggleButton value="request" sx={{ px: 1.5, py: 0.25, fontSize: '0.72rem' }}>{t('bench.request', { defaultValue: 'Request' })}</ToggleButton>
                    <ToggleButton value="curl" sx={{ px: 1.5, py: 0.25, fontSize: '0.72rem' }}>{t('probe.curl')}</ToggleButton>
                </ToggleButtonGroup>
                <Box sx={{ flex: 1 }} />
                <Typography variant="caption" sx={{ color: loading ? 'primary.main' : 'text.disabled', fontSize: '0.62rem' }}>
                    {loading ? t('bench.rebuilding', { defaultValue: 'Rebuilding… (500 ms debounce)' }) : t('bench.builtFrom', { defaultValue: 'Built from POST /api/v2/probe/curl — the same builders the run uses.' })}
                </Typography>
            </Box>
            {loading && !data && <LinearProgress sx={{ height: 3, borderRadius: 2 }} />}

            {curl && !curl.success && (
                <Alert severity="error" variant="outlined" sx={{ py: 0.25, fontSize: '0.78rem' }}>
                    <strong>{t('bench.buildFailed', { defaultValue: 'Could not build the request' })}</strong>
                    {curl.error?.message ? ` — ${curl.error.message}` : ''}
                </Alert>
            )}

            {data && tab === 'request' && (
                <Box sx={{ opacity: loading ? 0.6 : 1, transition: 'opacity .1s' }}>
                    <Box sx={{ fontFamily: 'monospace', fontSize: '0.78rem', display: 'flex', gap: 1, alignItems: 'baseline', wordBreak: 'break-all' }}>
                        <Box component="span" sx={{ color: 'success.main', fontWeight: 600, whiteSpace: 'nowrap' }}>{data.method}</Box>
                        <span>{data.url}</span>
                    </Box>

                    <Typography variant="overline" sx={{ display: 'flex', alignItems: 'center', gap: 1, fontSize: '0.6rem', mt: 1.5, '&::after': { content: '""', flex: 1, height: '1px', bgcolor: 'divider' } }}>
                        {t('bench.headers', { defaultValue: 'Headers' })}
                    </Typography>
                    <Box sx={{ display: 'grid', gridTemplateColumns: 'auto minmax(0, 1fr) 24px', gap: '2px 12px', fontFamily: 'monospace', fontSize: '0.72rem', alignItems: 'center' }}>
                        {headerRows.map(([name, value]) => {
                            const edited = overriddenHeader(name);
                            return (
                                <React.Fragment key={name}>
                                    <Box sx={{ color: 'text.disabled' }}>{name}</Box>
                                    <Box sx={{ wordBreak: 'break-all', color: edited ? 'primary.main' : 'text.primary' }}>{value}</Box>
                                    <Tooltip title={edited ? t('bench.removeOverride', { defaultValue: 'Remove override' }) : t('bench.removeHeader', { defaultValue: 'Remove header' })}>
                                        <IconButton size="small" sx={{ p: 0.25 }} onClick={() => (edited ? dropHeaderOverride(name) : setHeader(name, ''))} aria-label={`remove ${name}`}>
                                            <RemoveIcon sx={{ fontSize: 13 }} />
                                        </IconButton>
                                    </Tooltip>
                                </React.Fragment>
                            );
                        })}
                        {removedHeaders.map(([name]) => (
                            <React.Fragment key={`removed-${name}`}>
                                <Box sx={{ color: 'text.disabled', textDecoration: 'line-through' }}>{name}</Box>
                                <Box sx={{ color: 'warning.main' }}>{t('bench.headerRemoved', { defaultValue: 'removed' })}</Box>
                                <Tooltip title={t('bench.removeOverride', { defaultValue: 'Remove override' })}>
                                    <IconButton size="small" sx={{ p: 0.25 }} onClick={() => dropHeaderOverride(name)} aria-label={`restore ${name}`}>
                                        <RemoveIcon sx={{ fontSize: 13 }} />
                                    </IconButton>
                                </Tooltip>
                            </React.Fragment>
                        ))}
                    </Box>
                    {newHeader ? (
                        <Box sx={{ display: 'flex', gap: 1, mt: 1, alignItems: 'center' }}>
                            <TextField size="small" placeholder={t('bench.headerName', { defaultValue: 'Name' })} value={newHeader.name} onChange={(e) => setNewHeader({ ...newHeader, name: e.target.value })} slotProps={{ htmlInput: { sx: { fontSize: '0.72rem', py: 0.5, fontFamily: 'monospace' } } }} sx={{ width: 160 }} />
                            <TextField size="small" placeholder={t('bench.headerValue', { defaultValue: 'Value' })} value={newHeader.value} onChange={(e) => setNewHeader({ ...newHeader, value: e.target.value })} slotProps={{ htmlInput: { sx: { fontSize: '0.72rem', py: 0.5, fontFamily: 'monospace' } } }} sx={{ flex: 1 }} />
                            <Button size="small" variant="contained" disabled={!newHeader.name.trim() || !newHeader.value} onClick={() => { setHeader(newHeader.name.trim(), newHeader.value); setNewHeader(null); }}>
                                {t('bench.applyBody', { defaultValue: 'Apply' })}
                            </Button>
                            <Button size="small" onClick={() => setNewHeader(null)}>{t('bench.cancelEdit', { defaultValue: 'Cancel' })}</Button>
                        </Box>
                    ) : (
                        <Button size="small" sx={{ mt: 0.5, fontSize: '0.7rem', minWidth: 0 }} onClick={() => setNewHeader({ name: '', value: '' })}>
                            {t('bench.addHeader', { defaultValue: '+ Header' })}
                        </Button>
                    )}

                    <Typography variant="overline" sx={{ display: 'flex', alignItems: 'center', gap: 1, fontSize: '0.6rem', mt: 1.5, '&::after': { content: '""', flex: 1, height: '1px', bgcolor: 'divider' } }}>
                        {t('bench.body', { defaultValue: 'Body' })}
                        {!rawMode && (
                            <Tooltip title={t('bench.editBodyHint', { defaultValue: 'Take this body into the request editor as a raw request and change anything.' })}>
                                <Button size="small" onClick={() => onEditBody(pretty)} sx={{ minWidth: 0, fontSize: '0.65rem', py: 0, ml: 'auto' }}>
                                    {t('bench.editBody', { defaultValue: 'Edit' })}
                                </Button>
                            </Tooltip>
                        )}
                    </Typography>
                    <Box sx={{ position: 'relative' }}>
                        <Box component="pre" sx={{ m: 0, p: 1.5, pr: 5, bgcolor: 'background.default', borderRadius: 1.5, fontFamily: 'monospace', fontSize: '0.72rem', lineHeight: 1.5, overflow: 'auto', maxHeight: '60vh', color: 'text.primary' }}>
                            {pretty}
                        </Box>
                        <CopyIconButton value={pretty} label={t('probe.copy')} copiedLabel={t('probe.copied')} sx={{ position: 'absolute', top: 4, right: 4 }} />
                    </Box>
                </Box>
            )}

            {data && tab === 'curl' && (
                <Box sx={{ opacity: loading ? 0.6 : 1 }}>
                    <CopyBlock text={data.command} maxHeight="60vh" fontSize="0.72rem" />
                </Box>
            )}

            {data && (
                <Typography variant="caption" sx={{ color: 'text.secondary', lineHeight: 1.4 }}>
                    {t('bench.secretsHint', { key: data.key_env_var, defaultValue: 'Secrets stay as {{key}} — substitute before running by hand.' })}{' '}
                    {direct
                        ? t('bench.noteDirect', { defaultValue: 'Direct: the exact upstream request, authenticated with the provider key placeholder. No TB headers, no flags.' })
                        : t('bench.noteThroughTB', { defaultValue: "This is the request TB receives at its loopback entry: the protocol shape under test. Flags act after this point, inside TB — their effect shows in the response's Flags row." })}
                    {rawMode && (
                        <>
                            {' '}
                            {t('bench.noteRaw', { defaultValue: "Your own request, sent on its protocol's wire; only the model was filled in." })}
                        </>
                    )}
                </Typography>
            )}
        </Stack>
    );
};

export default PayloadPanel;
