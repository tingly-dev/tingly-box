import React, { useEffect, useMemo, useState } from 'react';
import { Alert, Box, Button, Chip, IconButton, LinearProgress, Stack, TextField, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from '@mui/material';
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
// the request that leaves the process (.design/playground.md §7).

const prettyJson = (raw?: string): string => {
    if (!raw) return '';
    try {
        return JSON.stringify(JSON.parse(raw), null, 2);
    } catch {
        return raw;
    }
};

// sjson path syntax treats "." as a separator; a literal dot in a key is escaped.
const escapePath = (key: string): string => key.replace(/\./g, '\\.');

// diffTopLevel folds the difference between the displayed body and the edited
// one into the existing overrides: changed keys are set, missing keys are
// deleted (null), untouched keys keep whatever override they already had.
export function diffTopLevel(
    displayed: Record<string, unknown>,
    edited: Record<string, unknown>,
    existing: Record<string, unknown>,
): Record<string, unknown> {
    const next = { ...existing };
    const keys = new Set([...Object.keys(displayed), ...Object.keys(edited)]);
    keys.forEach((k) => {
        const path = escapePath(k);
        if (!(k in edited)) next[path] = null;
        else if (JSON.stringify(displayed[k]) !== JSON.stringify(edited[k])) next[path] = edited[k];
    });
    return next;
}

export const PayloadPanel: React.FC<{
    request: ProbeRequest | null;
    curl: ProbeCurlResult | null;
    loading: boolean;
    direct: boolean;
    bodyOverrides: Record<string, unknown>;
    headers: Record<string, string>;
    onBodyOverridesChange: (next: Record<string, unknown>) => void;
    onHeadersChange: (next: Record<string, string>) => void;
}> = ({ request, curl, loading, direct, bodyOverrides, headers, onBodyOverridesChange, onHeadersChange }) => {
    const { t } = useTranslation();
    const [tab, setTab] = useState<'request' | 'curl'>('request');
    const [editing, setEditing] = useState(false);
    const [draft, setDraft] = useState('');
    const [draftError, setDraftError] = useState<string | null>(null);
    const [newHeader, setNewHeader] = useState<{ name: string; value: string } | null>(null);

    const data = curl?.success ? curl.data : undefined;
    const pretty = useMemo(() => prettyJson(data?.body), [data?.body]);
    const overrideKeys = Object.keys(bodyOverrides);
    const headerRows = useMemo(() => Object.entries(data?.headers ?? {}).sort(([a], [b]) => a.localeCompare(b)), [data?.headers]);
    const overriddenHeader = (name: string) => Object.keys(headers).some((h) => h.toLowerCase() === name.toLowerCase());
    const removedHeaders = Object.entries(headers).filter(([, v]) => v === '');

    // A rebuild while editing keeps the draft — the user's text is theirs
    // until they apply or cancel.
    useEffect(() => {
        if (!editing) setDraftError(null);
    }, [editing]);

    const startEdit = () => {
        setDraft(pretty);
        setDraftError(null);
        setEditing(true);
    };
    const applyEdit = () => {
        let parsed: unknown;
        try {
            parsed = JSON.parse(draft);
        } catch (e: any) {
            setDraftError(t('playground.bodyInvalid', { error: e?.message || 'parse error', defaultValue: 'Not valid JSON: {{error}}' }));
            return;
        }
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
            setDraftError(t('playground.bodyMustBeObject', { defaultValue: 'The body must be a JSON object.' }));
            return;
        }
        let displayed: Record<string, unknown> = {};
        try {
            displayed = JSON.parse(data?.body || '{}');
        } catch {
            displayed = {};
        }
        onBodyOverridesChange(diffTopLevel(displayed, parsed as Record<string, unknown>, bodyOverrides));
        setEditing(false);
    };
    const removeOverride = (key: string) => {
        const next = { ...bodyOverrides };
        delete next[key];
        onBodyOverridesChange(next);
    };
    const setHeader = (name: string, value: string) => onHeadersChange({ ...headers, [name]: value });
    const dropHeaderOverride = (name: string) => {
        const next = { ...headers };
        Object.keys(next).forEach((h) => { if (h.toLowerCase() === name.toLowerCase()) delete next[h]; });
        onHeadersChange(next);
    };

    if (!request) {
        return (
            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                {t('playground.noTarget', { defaultValue: 'Pick a target to see the payload.' })}
            </Typography>
        );
    }

    return (
        <Stack spacing={1.5}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <ToggleButtonGroup size="small" exclusive value={tab} onChange={(_, v) => v && setTab(v)}>
                    <ToggleButton value="request" sx={{ px: 1.5, py: 0.25, fontSize: '0.72rem' }}>{t('playground.request', { defaultValue: 'Request' })}</ToggleButton>
                    <ToggleButton value="curl" sx={{ px: 1.5, py: 0.25, fontSize: '0.72rem' }}>{t('probe.curl')}</ToggleButton>
                </ToggleButtonGroup>
                <Box sx={{ flex: 1 }} />
                <Typography variant="caption" sx={{ color: loading ? 'primary.main' : 'text.disabled', fontSize: '0.62rem' }}>
                    {loading ? t('playground.rebuilding', { defaultValue: 'Rebuilding… (500 ms debounce)' }) : t('playground.builtFrom', { defaultValue: 'Built from POST /api/v2/probe/curl — the same builders the run uses.' })}
                </Typography>
            </Box>
            {loading && !data && <LinearProgress sx={{ height: 3, borderRadius: 2 }} />}

            {curl && !curl.success && (
                <Alert severity="error" variant="outlined" sx={{ py: 0.25, fontSize: '0.78rem' }}>
                    <strong>{t('playground.buildFailed', { defaultValue: 'Could not build the request' })}</strong>
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
                        {t('playground.headers', { defaultValue: 'Headers' })}
                    </Typography>
                    <Box sx={{ display: 'grid', gridTemplateColumns: 'auto minmax(0, 1fr) 24px', gap: '2px 12px', fontFamily: 'monospace', fontSize: '0.72rem', alignItems: 'center' }}>
                        {headerRows.map(([name, value]) => {
                            const edited = overriddenHeader(name);
                            return (
                                <React.Fragment key={name}>
                                    <Box sx={{ color: 'text.disabled' }}>{name}</Box>
                                    <Box sx={{ wordBreak: 'break-all', color: edited ? 'primary.main' : 'text.primary' }}>{value}</Box>
                                    <Tooltip title={edited ? t('playground.removeOverride', { defaultValue: 'Remove override' }) : t('playground.removeHeader', { defaultValue: 'Remove header' })}>
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
                                <Box sx={{ color: 'warning.main' }}>{t('playground.headerRemoved', { defaultValue: 'removed' })}</Box>
                                <Tooltip title={t('playground.removeOverride', { defaultValue: 'Remove override' })}>
                                    <IconButton size="small" sx={{ p: 0.25 }} onClick={() => dropHeaderOverride(name)} aria-label={`restore ${name}`}>
                                        <RemoveIcon sx={{ fontSize: 13 }} />
                                    </IconButton>
                                </Tooltip>
                            </React.Fragment>
                        ))}
                    </Box>
                    {newHeader ? (
                        <Box sx={{ display: 'flex', gap: 1, mt: 1, alignItems: 'center' }}>
                            <TextField size="small" placeholder={t('playground.headerName', { defaultValue: 'Name' })} value={newHeader.name} onChange={(e) => setNewHeader({ ...newHeader, name: e.target.value })} slotProps={{ htmlInput: { sx: { fontSize: '0.72rem', py: 0.5, fontFamily: 'monospace' } } }} sx={{ width: 160 }} />
                            <TextField size="small" placeholder={t('playground.headerValue', { defaultValue: 'Value' })} value={newHeader.value} onChange={(e) => setNewHeader({ ...newHeader, value: e.target.value })} slotProps={{ htmlInput: { sx: { fontSize: '0.72rem', py: 0.5, fontFamily: 'monospace' } } }} sx={{ flex: 1 }} />
                            <Button size="small" variant="contained" disabled={!newHeader.name.trim() || !newHeader.value} onClick={() => { setHeader(newHeader.name.trim(), newHeader.value); setNewHeader(null); }}>
                                {t('playground.applyBody', { defaultValue: 'Apply' })}
                            </Button>
                            <Button size="small" onClick={() => setNewHeader(null)}>{t('playground.cancelEdit', { defaultValue: 'Cancel' })}</Button>
                        </Box>
                    ) : (
                        <Button size="small" sx={{ mt: 0.5, fontSize: '0.7rem', minWidth: 0 }} onClick={() => setNewHeader({ name: '', value: '' })}>
                            {t('playground.addHeader', { defaultValue: '+ Header' })}
                        </Button>
                    )}

                    <Typography variant="overline" sx={{ display: 'flex', alignItems: 'center', gap: 1, fontSize: '0.6rem', mt: 1.5, '&::after': { content: '""', flex: 1, height: '1px', bgcolor: 'divider' } }}>
                        {t('playground.body', { defaultValue: 'Body' })}
                        {!editing && (
                            <Button size="small" onClick={startEdit} sx={{ minWidth: 0, fontSize: '0.65rem', py: 0, ml: 'auto' }}>
                                {t('playground.editBody', { defaultValue: 'Edit' })}
                            </Button>
                        )}
                    </Typography>
                    {overrideKeys.length > 0 && (
                        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, alignItems: 'center', mb: 1 }}>
                            <Typography variant="caption" sx={{ color: 'primary.main', mr: 0.5 }}>
                                {t('playground.bodyOverridden', { count: overrideKeys.length, defaultValue: '{{count}} field(s) overridden by hand' })}
                            </Typography>
                            {overrideKeys.map((k) => (
                                <Chip key={k} size="small" color="primary" variant="outlined" label={bodyOverrides[k] === null ? `${k} ✕` : k} onDelete={() => removeOverride(k)} sx={{ fontFamily: 'monospace', fontSize: '0.65rem', height: 20 }} />
                            ))}
                            <Button size="small" onClick={() => onBodyOverridesChange({})} sx={{ minWidth: 0, fontSize: '0.65rem' }}>
                                {t('playground.resetBody', { defaultValue: 'Reset body' })}
                            </Button>
                        </Box>
                    )}
                    {editing ? (
                        <Stack spacing={1}>
                            <TextField
                                multiline
                                minRows={10}
                                maxRows={32}
                                value={draft}
                                onChange={(e) => setDraft(e.target.value)}
                                error={!!draftError}
                                helperText={draftError}
                                slotProps={{ htmlInput: { sx: { fontFamily: 'monospace', fontSize: '0.72rem', lineHeight: 1.5 } } }}
                            />
                            <Box sx={{ display: 'flex', gap: 1 }}>
                                <Button size="small" variant="contained" onClick={applyEdit}>{t('playground.applyBody', { defaultValue: 'Apply' })}</Button>
                                <Button size="small" onClick={() => setEditing(false)}>{t('playground.cancelEdit', { defaultValue: 'Cancel' })}</Button>
                            </Box>
                        </Stack>
                    ) : (
                        <Box sx={{ position: 'relative' }}>
                            <Box component="pre" sx={{ m: 0, p: 1.5, pr: 5, bgcolor: 'background.default', borderRadius: 1.5, fontFamily: 'monospace', fontSize: '0.72rem', lineHeight: 1.5, overflow: 'auto', maxHeight: '60vh', color: 'text.primary' }}>
                                {pretty}
                            </Box>
                            <CopyIconButton value={pretty} label={t('probe.copy')} copiedLabel={t('probe.copied')} sx={{ position: 'absolute', top: 4, right: 4 }} />
                        </Box>
                    )}
                </Box>
            )}

            {data && tab === 'curl' && (
                <Box sx={{ opacity: loading ? 0.6 : 1 }}>
                    <CopyBlock text={data.command} maxHeight="60vh" fontSize="0.72rem" />
                </Box>
            )}

            {data && (
                <Typography variant="caption" sx={{ color: 'text.secondary', lineHeight: 1.4 }}>
                    {t('playground.secretsHint', { key: data.key_env_var, defaultValue: 'Secrets stay as {{key}} — substitute before running by hand.' })}{' '}
                    {direct
                        ? t('playground.noteDirect', { defaultValue: 'Direct: the exact upstream request, authenticated with the provider key placeholder. No TB headers, no flags.' })
                        : t('playground.noteThroughTB', { defaultValue: "This is the request TB receives at its loopback entry: the protocol shape under test. Flags act after this point, inside TB — their effect shows in the response's Flags row." })}
                    {(overrideKeys.length > 0 || Object.keys(headers).length > 0) && (
                        <>
                            {' '}
                            {t('playground.noteOverrides', { defaultValue: 'Hand edits become overrides applied after every builder and flag — by the run and by this panel alike, so the two cannot disagree.' })}
                        </>
                    )}
                </Typography>
            )}
        </Stack>
    );
};

export default PayloadPanel;
