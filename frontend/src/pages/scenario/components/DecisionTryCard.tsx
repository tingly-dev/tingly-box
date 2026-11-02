import { useEffect, useMemo, useState } from 'react';
import { Alert, Box, Button, CircularProgress, LinearProgress, Stack, Tab, Tabs, TextField, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';
import { PlayArrow } from '@/components/icons';
import { toggleButtonGroupStyle } from '@/styles/toggleStyles';
import { useTranslation } from 'react-i18next';
import type { Rule } from '@/components/RoutingGraphTypes';
import type { ProbeResult } from '@/types/probe';
import { runProbe, formatLatency } from '@/components/probe/runProbe';
import { CollapsibleCard } from '@/components/CollapsibleCard';
import ImageModelPicker from '@/pages/image/components/ImageModelPicker';
import CodeBlock from '@/components/CodeBlock';

type QuestionType = 'choice' | 'noul' | 'score';
interface DecisionAnswer {
    type?: string;
    choice?: string;
    noul?: number;
    score?: number;
    confidence?: number;
    probabilities?: Record<string, number>;
    legend?: Record<string, string>;
}

/** A single real decision through the selected scenario's normal routing. */
export default function DecisionTryCard({ rules, showNotification, onRulesReload }: {
    rules: Rule[];
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    onRulesReload: () => Promise<void>;
}) {
    const { t } = useTranslation();
    const available = useMemo(() => rules.filter(rule => rule.uuid && rule.active !== false && rule.request_model), [rules]);
    const [selectedId, setSelectedId] = useState('');
    const selected = available.find(rule => rule.uuid === selectedId) ?? available[0];
    const [wildcardModel, setWildcardModel] = useState('');
    const [state, setState] = useState(() => t('decisionTry.exampleInput'));
    const [question, setQuestion] = useState(() => t('decisionTry.exampleQuestion'));
    const [kind, setKind] = useState<QuestionType>('choice');
    const [options, setOptions] = useState(() => t('decisionTry.exampleOptions'));
    const [running, setRunning] = useState(false);
    const [result, setResult] = useState<ProbeResult | null>(null);
    const [stale, setStale] = useState(false);
    const [expanded, setExpanded] = useState(false);
    const [tab, setTab] = useState<'answer' | 'raw'>('answer');
    const values = options.split('\n').map(value => value.trim()).filter(Boolean);
    const needsOptions = kind !== 'noul';
    const maxOptions = kind === 'score' ? 10 : 255;
    const validOptions = !needsOptions || (values.length >= 2 && values.length <= maxOptions && new Set(values).size === values.length);
    const model = selected?.request_model === '*' ? wildcardModel.trim() : selected?.request_model;
    const canRun = !!selected && !!model && model !== '*' && !!state.trim() && !!question.trim() && validOptions && !running;
    useEffect(() => { setResult(null); setStale(false); }, [selected?.uuid]);
    const change = (action: () => void) => {
        if (result?.success) setStale(true);
        else if (result) setResult(null);
        action();
    };
    const changeKind = (next: QuestionType) => change(() => {
        const examples = [t('decisionTry.exampleQuestion'), t('decisionTry.examplePredicate'), t('decisionTry.exampleScore')];
        if (examples.includes(question)) setQuestion(next === 'noul' ? examples[1] : next === 'score' ? examples[2] : examples[0]);
        if ([t('decisionTry.exampleOptions'), t('decisionTry.exampleLevels')].includes(options)) {
            setOptions(next === 'score' ? t('decisionTry.exampleLevels') : t('decisionTry.exampleOptions'));
        }
        setKind(next);
    });

    const run = async () => {
        if (!selected || !canRun) return;
        const q: Record<string, unknown> = { type: kind, instructions: question };
        if (kind === 'choice') q.criteria = Object.fromEntries(values.map(value => [value, '']));
        if (kind === 'score') q.criteria = values;
        setRunning(true);
        setResult(null);
        setStale(false);
        setTab('answer');
        try {
            const response = await runProbe({ target_type: 'rule', scenario: 'decisions', rule_uuid: selected.uuid,
                operation: 'decisions', ...(selected.request_model === '*' ? { model } : {}),
                decision_request: { state, questions: { result: q } } });
            setResult(response);
        } finally {
            setRunning(false);
        }
    };

    const raw = result?.data?.decision_response;
    const answers = raw?.answers as Record<string, DecisionAnswer> | undefined;
    const answer = answers?.result;
    const percent = (value: number) => `${(value * 100).toFixed(1)}%`;
    const mismatch = !!result?.data?.matched_rule && result.data.matched_rule !== selected?.uuid;

    const probabilities = answer?.probabilities
        ? Object.entries(answer.probabilities).filter(([, value]) => typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= 1)
        : [];
    // The most likely outcome leads the breakdown; the rest stay muted.
    const topProbability = probabilities.length > 0
        ? probabilities.reduce((top, entry) => (entry[1] > top[1] ? entry : top))
        : null;

    return (
        <CollapsibleCard
            title={t('decisionTry.title')}
            description={t('decisionTry.subtitle')}
            expanded={expanded}
            onToggle={() => setExpanded(value => !value)}
        >
            {!selected ? (
                <Alert severity="info">{t('decisionTry.noRules')}</Alert>
            ) : (
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', md: 'minmax(0, 1fr) minmax(0, 1fr)' }, gap: { xs: 2, md: 4 } }}>
                    <Stack spacing={1.5} sx={{ minWidth: 0 }}>
                        <ImageModelPicker
                            scenario="decisions"
                            label={t('decisionTry.rule')}
                            rules={rules}
                            value={selected.request_model ?? ''}
                            onChange={(requestModel) => change(() => {
                                const rule = rules.find(item => item.request_model?.trim() === requestModel);
                                if (rule?.uuid) setSelectedId(rule.uuid);
                            })}
                            showNotification={showNotification}
                            onRulesReload={onRulesReload}
                        />
                        {selected.request_model === '*' && <TextField fullWidth size="small" label={t('decisionTry.model')} value={wildcardModel} disabled={running}
                            onChange={e => change(() => setWildcardModel(e.target.value))} helperText={t('decisionTry.concreteModel')} />}
                        <TextField fullWidth label={t('decisionTry.input')} value={state} multiline minRows={3} maxRows={8} disabled={running}
                            onChange={e => change(() => setState(e.target.value))} />
                        <Box>
                            <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500, display: 'block', mb: 0.5 }}>{t('decisionTry.type')}</Typography>
                            <ToggleButtonGroup exclusive fullWidth size="small" aria-label={t('decisionTry.type')} value={kind} disabled={running}
                                onChange={(_, next: QuestionType | null) => { if (next) changeKind(next); }}
                                sx={{ ...toggleButtonGroupStyle, '& .MuiToggleButton-root': { ...((toggleButtonGroupStyle as Record<string, any>)['& .MuiToggleButton-root'] ?? {}), flex: 1, px: 1, height: 'auto', minHeight: 32, fontSize: '0.8125rem', lineHeight: 1.3 } }}>
                                <ToggleButton value="choice">{t('decisionTry.choice')}</ToggleButton>
                                <ToggleButton value="noul">{t('decisionTry.predicate')}</ToggleButton>
                                <ToggleButton value="score">{t('decisionTry.score')}</ToggleButton>
                            </ToggleButtonGroup>
                        </Box>
                        <TextField fullWidth size="small" label={t('decisionTry.question')} value={question} disabled={running}
                            onChange={e => change(() => setQuestion(e.target.value))} />
                        {needsOptions && <TextField fullWidth size="small" label={kind === 'score' ? t('decisionTry.levels') : t('decisionTry.options')}
                            value={options} multiline minRows={2} maxRows={8} disabled={running} error={!validOptions}
                            helperText={validOptions ? t('decisionTry.onePerLine') : t('decisionTry.invalidOptions', { max: maxOptions })}
                            onChange={e => change(() => setOptions(e.target.value))} />}
                        <Box>
                            <Button variant="contained" size="small" startIcon={running ? <CircularProgress size={16} color="inherit" /> : <PlayArrow fontSize="small" />}
                                onClick={() => void run()} disabled={!canRun}>
                                {running ? t('decisionTry.running') : t('decisionTry.run')}
                            </Button>
                        </Box>
                    </Stack>

                    <Box component="section" aria-label={t('decisionTry.result')} aria-live="polite" sx={{ minWidth: 0, maxHeight: 360, overflowY: 'auto' }}>
                        {running ? (
                            <Stack spacing={1.5} sx={{ mt: 0.5 }}>
                                <LinearProgress />
                                <Typography variant="body2" color="text.secondary">{t('decisionTry.runningHint')}</Typography>
                            </Stack>
                        ) : !result ? (
                            <Stack spacing={0.5} sx={{ alignItems: 'center', justifyContent: 'center', minHeight: 140, px: 3, py: 3, textAlign: 'center', border: '1px dashed', borderColor: 'divider', borderRadius: 2 }}>
                                <Typography variant="body2" sx={{ fontWeight: 600 }}>{t('decisionTry.emptyTitle')}</Typography>
                                <Typography variant="caption" color="text.secondary">{t('decisionTry.emptyBody')}</Typography>
                            </Stack>
                        ) : !result.success ? (
                            <Alert severity="error">{result.error?.message || t('decisionTry.failed')}</Alert>
                        ) : !answer ? (
                            <Alert severity="error">{t('decisionTry.missingAnswer')}</Alert>
                        ) : (
                            <Stack spacing={1.5} sx={{ mt: 0.5 }}>
                                {stale && <Alert severity="info">{t('decisionTry.stale')}</Alert>}
                                <Tabs value={tab} onChange={(_, next: 'answer' | 'raw') => setTab(next)} sx={{ minHeight: 'auto', '& .MuiTab-root': { minHeight: 36, py: 0.5 } }}>
                                    <Tab value="answer" label={t('decisionTry.answer')} />
                                    <Tab value="raw" label={t('decisionTry.raw')} />
                                </Tabs>
                                {tab === 'answer' && (
                                    <Stack spacing={2}>
                                        <Box sx={{ p: 1.5, bgcolor: 'action.hover', borderRadius: 1.5 }}>
                                            <Typography variant="caption" color="text.secondary">{t('decisionTry.answer')}</Typography>
                                            {answer.type === 'choice' && <Typography variant="h5" sx={{ fontWeight: 600, overflowWrap: 'anywhere' }}>{answer.choice}</Typography>}
                                            {answer.type === 'noul' && typeof answer.noul === 'number' && <Typography variant="h5" sx={{ fontWeight: 600 }}>{percent(answer.noul)}</Typography>}
                                            {answer.type === 'score' && typeof answer.score === 'number' && <Typography variant="h5" sx={{ fontWeight: 600 }}>{answer.score.toFixed(2)}</Typography>}
                                        </Box>
                                        {probabilities.length > 0 && (
                                            <Stack spacing={1.5}>
                                                <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>{t('decisionTry.probabilities')}</Typography>
                                                {probabilities.map(([key, value]) => {
                                                    const isTop = topProbability !== null && key === topProbability[0] && topProbability[1] > 0;
                                                    return (
                                                        <Box key={key}>
                                                            <Stack direction="row" sx={{ justifyContent: 'space-between', gap: 2, mb: 0.5 }}>
                                                                <Typography variant="body2" sx={{ overflowWrap: 'anywhere', minWidth: 0, ...(isTop && { fontWeight: 600 }) }}>
                                                                    {answer.legend?.[key] ?? key}
                                                                </Typography>
                                                                <Typography variant="body2" sx={{ flexShrink: 0, fontVariantNumeric: 'tabular-nums', ...(isTop && { fontWeight: 600 }) }}>
                                                                    {percent(value)}
                                                                </Typography>
                                                            </Stack>
                                                            <LinearProgress variant="determinate" value={value * 100}
                                                                sx={{ height: 6, borderRadius: 3, ...(isTop ? {} : { '& .MuiLinearProgress-bar': { backgroundColor: 'text.disabled' } }) }} />
                                                        </Box>
                                                    );
                                                })}
                                            </Stack>
                                        )}
                                        {typeof answer.confidence === 'number' && <Typography variant="body2" color="text.secondary">{t('decisionTry.confidence')}: {percent(answer.confidence)}</Typography>}
                                        {mismatch && <Alert severity="warning">{t('decisionTry.differentRule')}</Alert>}
                                        <Typography variant="body2" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>
                                            {[result.data?.selected_provider, result.data?.selected_model, result.data?.latency_ms != null ? formatLatency(result.data.latency_ms) : undefined].filter(Boolean).join(' · ')}
                                        </Typography>
                                    </Stack>
                                )}
                                {tab === 'raw' && (
                                    <CodeBlock code={JSON.stringify(raw, null, 2)} language="json" maxHeight={280} wrap sx={{ mt: 1 }} />
                                )}
                            </Stack>
                        )}
                    </Box>
                </Box>
            )}
        </CollapsibleCard>
    );
}
