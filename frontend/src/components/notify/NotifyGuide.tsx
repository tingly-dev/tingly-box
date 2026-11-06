import CodeBlock from '@/components/CodeBlock';
import {CheckCircle, Send as SendIcon} from '@/components/icons';
import {
    BOT_PLACEHOLDER,
    SAMPLE_LABELS,
    SAMPLE_LANGS,
    SAMPLE_MESSAGE,
    SAMPLE_PRISM,
    SAMPLE_TOKEN_ENV,
    TARGET_PLACEHOLDER,
    buildSample,
    type NotifyPayload,
    type SampleLang,
} from '@/components/notify/notifySamples';
import {loadNotifyTargets, notifyTargetLabel, notifyTargetParts} from '@/components/notify/notifyTargets';
import {api} from '@/services/api';
import {fontMono, fontSizes} from '@/theme/fonts';
import type {BotSettings, NotifyTarget} from '@/types/bot';
import {capabilityEnabled, isPairingRequired} from '@/types/bot';
import {getApiBaseUrl} from '@/utils/protocol';
import {
    Alert,
    Autocomplete,
    Box,
    Button,
    CircularProgress,
    Stack,
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import type {ReactNode} from 'react';
import {useEffect, useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';

// NotifyGuide teaches an operator how to call POST /api/v1/bots/{bot}/notify,
// built around the one decision that is theirs — where to deliver — instead of
// a wall of prose (ux-principles #1, #5, #11):
//
//   1. Notify is on        (state, not instructions)
//   2. Pick a target       (friendly label; the internal UUID is filled in)
//   3. Send it             (one sample in four languages + a live Send test)
//
// The request fields are a reference list under the steps, not `//` comments
// inside the JSON. Auth reuses the operator's user token (no new credential,
// .design/bot-interaction-api.md §3.1); samples read it from $TINGLY_TOKEN so
// a secret never lands in the page. The component is width-agnostic: it is the
// body of the Bots-page dialog (wide) and of a drawer (narrow), and switches to
// two columns by container width, not viewport.
export interface NotifyGuideProps {
    /** Bots to offer targets from; only those with Notify on contribute. */
    bots: BotSettings[];
}

interface TargetOption {
    key: string;
    bot: BotSettings;
    target: NotifyTarget;
}

type SendState =
    | {status: 'idle'}
    | {status: 'sending'}
    | {status: 'delivered'; latencyMs: number}
    | {status: 'failed'; error: string};

const formatLatency = (ms: number): string => (ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`);

const STEP_AREAS = {enable: 'enable', target: 'target', send: 'send', fields: 'fields'} as const;

const Step: React.FC<{n: number; title: string; area: string; children: ReactNode}> = ({n, title, area, children}) => (
    <Box component="section" sx={{gridArea: area, minWidth: 0}}>
        <Stack direction="row" spacing={1} sx={{alignItems: 'center', mb: 1}}>
            <Box
                aria-hidden
                sx={{
                    width: 22, height: 22, flexShrink: 0, borderRadius: '50%',
                    bgcolor: 'primary.main', color: 'primary.contrastText',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    fontSize: fontSizes.sm, fontWeight: 600,
                }}
            >
                {n}
            </Box>
            <Typography variant="subtitle2" component="h3" sx={{fontWeight: 600}}>{title}</Typography>
        </Stack>
        {children}
    </Box>
);

const NotifyGuide: React.FC<NotifyGuideProps> = ({bots}) => {
    const {t} = useTranslation();

    // The gateway's own URL — in the desktop window the page origin is
    // wails://, which curl can't reach.
    const [origin, setOrigin] = useState(window.location.origin);
    useEffect(() => {
        let cancelled = false;
        getApiBaseUrl().then((url) => { if (!cancelled) setOrigin(url); });
        return () => { cancelled = true; };
    }, []);

    const notifyBots = useMemo(
        () => bots.filter((bot) => bot.uuid && (bot.enabled ?? true) && capabilityEnabled(bot, 'notify')),
        [bots],
    );
    const botKey = notifyBots.map((bot) => bot.uuid).join(',');

    const [options, setOptions] = useState<TargetOption[]>([]);
    const [loading, setLoading] = useState(false);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [selectedKey, setSelectedKey] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        if (notifyBots.length === 0) {
            setOptions([]);
            setLoadError(null);
            return;
        }
        setLoading(true);
        setLoadError(null);
        Promise.all(notifyBots.map(async (bot) => {
            const targets = await loadNotifyTargets(bot.uuid!);
            return targets
                .filter((target) => !target.blocked)
                .map((target): TargetOption => ({key: `${bot.uuid}:${target.id}`, bot, target}));
        }))
            .then((groups) => { if (!cancelled) setOptions(groups.flat()); })
            .catch((error) => { if (!cancelled) setLoadError((error as Error).message); })
            .finally(() => { if (!cancelled) setLoading(false); });
        return () => { cancelled = true; };
        // botKey is the stable identity of notifyBots.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [botKey]);

    // Default to the first target that can actually receive, so the sample is
    // runnable the moment the guide opens.
    const selected = options.find((option) => option.key === selectedKey)
        ?? options.find((option) => option.target.can_notify)
        ?? options[0]
        ?? null;

    const payload: NotifyPayload = {
        target: {kind: selected?.target.kind ?? 'direct_chat', id: selected?.target.id ?? TARGET_PLACEHOLDER},
        ...SAMPLE_MESSAGE,
    };
    const botID = selected?.bot.uuid ?? BOT_PLACEHOLDER;

    const [lang, setLang] = useState<SampleLang>('curl');
    const sample = buildSample(lang, origin, botID, payload);

    const [send, setSend] = useState<SendState>({status: 'idle'});
    // A result belongs to the target it was sent to; picking another clears it.
    useEffect(() => setSend({status: 'idle'}), [selected?.key]);

    const handleSend = async () => {
        if (!selected) return;
        setSend({status: 'sending'});
        const started = Date.now();
        const result = await api.notifyBot(selected.bot.uuid!, payload);
        setSend(result.error
            ? {status: 'failed', error: result.error}
            : {status: 'delivered', latencyMs: Date.now() - started});
    };

    const multiBot = notifyBots.length > 1;
    // A chat only exists once it has messaged the bot (and paired, when the bot
    // requires it), so an empty list says exactly what to do next.
    const only = notifyBots.length === 1 ? notifyBots[0] : null;
    const platform = only?.platform || 'its platform';
    const emptyText = only && isPairingRequired(only)
        ? t('notify.group.emptyPairFirst', {defaultValue: 'No chats yet. Pair this bot, then send it a message on {{platform}} — its Chat ID appears here.', platform})
        : only
            ? t('notify.group.empty', {defaultValue: 'No chats yet. Send any message to this bot on {{platform}} and its Chat ID appears here.', platform})
            : t('notify.guide.target.empty', {defaultValue: 'No chats yet. Send any message to the bot on its platform and it appears here.'});
    const canSend = Boolean(selected?.target.can_notify) && send.status !== 'sending';

    const fields: Array<{name: string; type: string; required: boolean; desc: string}> = [
        {name: 'target.kind', type: 'string', required: true, desc: t('notify.guide.fields.kind', {defaultValue: 'direct_chat or group — filled in from the target you picked.'})},
        {name: 'target.id', type: 'string', required: true, desc: t('notify.guide.fields.id', {defaultValue: 'Internal id of the delivery target — filled in from the target you picked.'})},
        {name: 'title', type: 'string', required: false, desc: t('notify.guide.fields.title', {defaultValue: 'Short headline shown above the message.'})},
        {name: 'body', type: 'string', required: true, desc: t('notify.guide.fields.body', {defaultValue: 'The message text. Markdown is supported.'})},
        {name: 'level', type: 'info | warn | error', required: false, desc: t('notify.guide.fields.level', {defaultValue: 'Severity hint: info, warn or error.'})},
    ];

    return (
        // Container-type makes the two-column switch follow the width this
        // guide is given (dialog vs drawer), not the window's.
        <Box sx={{containerType: 'inline-size', minWidth: 0}}>
            <Box
                sx={{
                    display: 'grid',
                    gap: 3,
                    minWidth: 0,
                    gridTemplateColumns: 'minmax(0, 1fr)',
                    gridTemplateAreas: `"${STEP_AREAS.enable}" "${STEP_AREAS.target}" "${STEP_AREAS.send}" "${STEP_AREAS.fields}"`,
                    '@container (min-width: 760px)': {
                        gridTemplateColumns: 'minmax(0, 5fr) minmax(0, 7fr)',
                        gridTemplateRows: 'auto auto 1fr',
                        gridTemplateAreas: `"${STEP_AREAS.enable} ${STEP_AREAS.send}" "${STEP_AREAS.target} ${STEP_AREAS.send}" "${STEP_AREAS.fields} ${STEP_AREAS.send}"`,
                        columnGap: 4,
                    },
                }}
            >
                <Step n={1} area={STEP_AREAS.enable} title={t('notify.guide.enable.title', {defaultValue: 'Turn Notify on'})}>
                    {notifyBots.length > 0 ? (
                        <Stack direction="row" spacing={0.75} sx={{alignItems: 'center', color: 'success.main'}}>
                            <CheckCircle fontSize="small"/>
                            <Typography variant="body2" sx={{minWidth: 0, overflowWrap: 'anywhere'}}>
                                {t('notify.guide.enable.on', {
                                    defaultValue: 'Notify is on for {{bots}}.',
                                    bots: notifyBots.map((bot) => bot.name || bot.platform).join(', '),
                                })}
                            </Typography>
                        </Stack>
                    ) : (
                        <Alert severity="warning" variant="outlined">
                            {bots.length === 0
                                ? t('notify.guide.enable.noBots', {defaultValue: 'No bot is connected yet. Connect one on the Bots page first.'})
                                : t('notify.guide.enable.off', {defaultValue: 'Notify is off. Turn it on with the switch on the bot card, then come back to this guide.'})}
                        </Alert>
                    )}
                </Step>

                <Step n={2} area={STEP_AREAS.target} title={t('notify.guide.target.title', {defaultValue: 'Pick where to deliver'})}>
                    <Autocomplete
                        size="small"
                        fullWidth
                        disableClearable
                        disabled={notifyBots.length === 0}
                        loading={loading}
                        options={options}
                        value={selected as TargetOption}
                        groupBy={multiBot ? (option) => option.bot.name || option.bot.platform || '' : undefined}
                        getOptionKey={(option) => option.key}
                        getOptionLabel={(option) => (option ? notifyTargetLabel(option.target, t) : '')}
                        isOptionEqualToValue={(a, b) => a.key === b.key}
                        onChange={(_event, option) => setSelectedKey(option.key)}
                        noOptionsText={emptyText}
                        renderOption={(props, option) => {
                            const {key, ...optionProps} = props as typeof props & {key: string};
                            const parts = notifyTargetParts(option.target, t);
                            return (
                                <li key={key} {...optionProps}>
                                    <Box sx={{minWidth: 0}}>
                                        <Typography variant="body2" sx={{fontWeight: 600, color: 'text.primary', overflowWrap: 'anywhere'}}>
                                            {[parts.platform, parts.name || parts.id].filter(Boolean).join(' · ')}
                                        </Typography>
                                        <Typography variant="caption" sx={{color: 'text.secondary', display: 'block', fontFamily: fontMono, overflowWrap: 'anywhere'}}>
                                            {[parts.kind, parts.name ? parts.id : ''].filter(Boolean).join(' · ')}
                                            {!option.target.can_notify && (
                                                <Box component="span" sx={{color: 'warning.main', fontFamily: 'inherit'}}>
                                                    {' · '}{t('notify.guide.target.notAllowed', {defaultValue: 'not allowed yet'})}
                                                </Box>
                                            )}
                                        </Typography>
                                    </Box>
                                </li>
                            );
                        }}
                        renderInput={(params) => (
                            <TextField
                                {...params}
                                label={t('notify.guide.target.label', {defaultValue: 'Delivery target'})}
                                error={Boolean(loadError)}
                                helperText={loadError
                                    ?? (selected && !selected.target.can_notify
                                        ? t('notify.guide.target.notAllowedHint', {defaultValue: 'This target can’t receive notifications yet. Use “Allow Notify & Test” on the bot card first.'})
                                        : undefined)}
                                slotProps={{
                                    ...params.slotProps,
                                    input: {
                                        ...params.slotProps.input,
                                        endAdornment: (<>{loading && <CircularProgress size={16}/>}{params.slotProps.input.endAdornment}</>),
                                    },
                                }}
                            />
                        )}
                    />
                    {notifyBots.length > 0 && !loading && options.length === 0 && !loadError && (
                        <Typography variant="caption" sx={{display: 'block', mt: 0.75, color: 'text.secondary'}}>
                            {emptyText}
                        </Typography>
                    )}
                </Step>

                <Step n={3} area={STEP_AREAS.send} title={t('notify.guide.send.title', {defaultValue: 'Send it'})}>
                    <Typography variant="body2" sx={{color: 'text.secondary', mb: 1.5}}>
                        {t('notify.guide.send.auth', {
                            defaultValue: 'Authenticate with your user token — the one this web UI uses — as a Bearer header. The samples read it from ${{env}}.',
                            env: SAMPLE_TOKEN_ENV,
                        })}
                    </Typography>
                    <Tabs
                        value={lang}
                        onChange={(_event, value: SampleLang) => setLang(value)}
                        variant="scrollable"
                        scrollButtons="auto"
                        sx={{minHeight: 36, mb: 1, '& .MuiTab-root': {minHeight: 36, textTransform: 'none', px: 1.5}}}
                    >
                        {SAMPLE_LANGS.map((value) => (
                            <Tab
                                key={value}
                                value={value}
                                label={SAMPLE_LABELS[value]}
                            />
                        ))}
                    </Tabs>
                    <CodeBlock code={sample} language={SAMPLE_PRISM[lang]} wrap maxHeight={360} sx={{minWidth: 0}}/>
                    <Stack direction="row" spacing={1.5} sx={{alignItems: 'center', mt: 1.5, flexWrap: 'wrap', rowGap: 1}}>
                        <Button
                            variant="contained"
                            size="small"
                            disabled={!canSend}
                            onClick={() => void handleSend()}
                            startIcon={send.status === 'sending' ? <CircularProgress size={14} color="inherit"/> : <SendIcon fontSize="small"/>}
                        >
                            {t('notify.guide.send.test', {defaultValue: 'Send test'})}
                        </Button>
                        <Typography variant="caption" sx={{color: 'text.secondary', flex: 1, minWidth: 160}}>
                            {t('notify.guide.send.testHint', {defaultValue: 'Sends exactly the request above, with this web UI’s token.'})}
                        </Typography>
                    </Stack>
                    {send.status === 'delivered' && (
                        <Alert severity="success" variant="outlined" sx={{mt: 1.5}}>
                            {t('notify.guide.send.delivered', {defaultValue: 'Delivered · {{latency}}', latency: formatLatency(send.latencyMs)})}
                        </Alert>
                    )}
                    {send.status === 'failed' && (
                        <Alert severity="error" variant="outlined" sx={{mt: 1.5, overflowWrap: 'anywhere'}}>
                            {t('notify.guide.send.failed', {defaultValue: 'Failed: {{error}}', error: send.error})}
                        </Alert>
                    )}
                </Step>

                <Box component="section" sx={{gridArea: STEP_AREAS.fields, minWidth: 0}}>
                    <Typography variant="subtitle2" component="h3" sx={{fontWeight: 600, mb: 1}}>
                        {t('notify.guide.fields.heading', {defaultValue: 'Request fields'})}
                    </Typography>
                    <Stack spacing={1} divider={<Box sx={{borderTop: 1, borderColor: 'divider'}}/>}>
                        {fields.map((field) => (
                            <Box key={field.name} sx={{pt: 0.25}}>
                                <Stack direction="row" spacing={1} sx={{alignItems: 'baseline', flexWrap: 'wrap', columnGap: 1}}>
                                    <Typography component="code" sx={{fontFamily: fontMono, fontSize: fontSizes.sm, fontWeight: 600}}>
                                        {field.name}
                                    </Typography>
                                    <Typography variant="caption" sx={{color: 'text.secondary', fontFamily: fontMono}}>{field.type}</Typography>
                                    <Typography variant="caption" sx={{color: field.required ? 'warning.main' : 'text.disabled'}}>
                                        {field.required
                                            ? t('notify.guide.fields.required', {defaultValue: 'required'})
                                            : t('notify.guide.fields.optional', {defaultValue: 'optional'})}
                                    </Typography>
                                </Stack>
                                <Typography variant="body2" sx={{color: 'text.secondary'}}>{field.desc}</Typography>
                            </Box>
                        ))}
                    </Stack>
                    <Typography variant="caption" sx={{display: 'block', mt: 1.5, color: 'text.secondary'}}>
                        {t('notify.guide.fields.responses', {defaultValue: '200 delivered · 400 invalid body · 404 bot not running or target not reachable · 500 delivery failed'})}
                    </Typography>
                </Box>
            </Box>
        </Box>
    );
};

export default NotifyGuide;
