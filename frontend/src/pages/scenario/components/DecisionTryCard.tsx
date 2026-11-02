import { useEffect, useMemo, useState } from 'react';
import { Accordion, AccordionDetails, AccordionSummary, Alert, Box, Button, LinearProgress, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { ExpandMore } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import type { Rule } from '@/components/RoutingGraphTypes';
import type { ProbeResult } from '@/types/probe';
import { runProbe, formatLatency } from '@/components/probe/runProbe';

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
export default function DecisionTryCard({ rules }: { rules: Rule[] }) {
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
    const values = options.split('\n').map(value => value.trim()).filter(Boolean);
    const needsOptions = kind !== 'noul';
    const maxOptions = kind === 'score' ? 10 : 255;
    const validOptions = !needsOptions || (values.length >= 2 && values.length <= maxOptions && new Set(values).size === values.length);
    const model = selected?.request_model === '*' ? wildcardModel.trim() : selected?.request_model;
    const canRun = !!selected && !!model && model !== '*' && !!state.trim() && !!question.trim() && validOptions && !running;
    useEffect(() => { setResult(null); }, [selected?.uuid]);
    const change = (action: () => void) => { setResult(null); action(); };
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

    return (
        <Accordion disableGutters defaultExpanded sx={{ border: 1, borderColor: 'divider', borderRadius: 2, '&:before': { display: 'none' } }}>
            <AccordionSummary expandIcon={<ExpandMore />}>
                <Box><Typography variant="subtitle1" sx={{ fontWeight: 600 }}>{t('decisionTry.title')}</Typography>
                    <Typography variant="body2" color="text.secondary">{t('decisionTry.subtitle')}</Typography></Box>
            </AccordionSummary>
            <AccordionDetails>
                {!selected ? <Alert severity="info">{t('decisionTry.noRules')}</Alert> : (
                    <Stack spacing={2} sx={{ maxWidth: 760 }}>
                        <TextField select size="small" label={t('decisionTry.rule')} value={selected.uuid} disabled={running}
                            onChange={e => change(() => setSelectedId(e.target.value))}>
                            {available.map(rule => <MenuItem key={rule.uuid} value={rule.uuid}>{rule.request_model}{rule.description ? ` · ${rule.description}` : ''}</MenuItem>)}
                        </TextField>
                        {selected.request_model === '*' && <TextField size="small" label={t('decisionTry.model')} value={wildcardModel} disabled={running}
                            onChange={e => change(() => setWildcardModel(e.target.value))} helperText={t('decisionTry.concreteModel')} />}
                        <TextField label={t('decisionTry.input')} value={state} multiline minRows={2} disabled={running}
                            onChange={e => change(() => setState(e.target.value))} />
                        <TextField select size="small" label={t('decisionTry.type')} value={kind} disabled={running}
                            onChange={e => changeKind(e.target.value as QuestionType)}>
                            <MenuItem value="choice">{t('decisionTry.choice')}</MenuItem><MenuItem value="noul">{t('decisionTry.predicate')}</MenuItem><MenuItem value="score">{t('decisionTry.score')}</MenuItem>
                        </TextField>
                        <TextField label={t('decisionTry.question')} value={question} disabled={running}
                            onChange={e => change(() => setQuestion(e.target.value))} />
                        {needsOptions && <TextField label={kind === 'score' ? t('decisionTry.levels') : t('decisionTry.options')}
                            value={options} multiline minRows={2} disabled={running} error={!validOptions}
                            helperText={validOptions ? t('decisionTry.onePerLine') : t('decisionTry.invalidOptions', { max: maxOptions })}
                            onChange={e => change(() => setOptions(e.target.value))} />}
                        <Box><Button variant="contained" onClick={() => void run()} disabled={!canRun}>{running ? t('decisionTry.running') : t('decisionTry.run')}</Button></Box>
                        {result && !result.success && <Alert severity="error">{result.error?.message || t('decisionTry.failed')}</Alert>}
                        {result?.success && !answer && <Alert severity="error">{t('decisionTry.missingAnswer')}</Alert>}
                        {result?.success && answer && (
                            <Box aria-live="polite">
                                <Typography variant="subtitle2">{t('decisionTry.result')}</Typography>
                                {answer.type === 'choice' && <Typography variant="h6">{answer.choice}</Typography>}
                                {answer.type === 'noul' && typeof answer.noul === 'number' && <Typography variant="h6">{percent(answer.noul)}</Typography>}
                                {answer.type === 'score' && typeof answer.score === 'number' && <Typography variant="h6">{answer.score.toFixed(2)}</Typography>}
                                {answer.probabilities && Object.entries(answer.probabilities).filter(([, value]) => typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= 1).map(([key, value]) => (
                                    <Box key={key} sx={{ mt: 1 }}><Stack direction="row" sx={{ justifyContent: 'space-between' }}><Typography variant="body2">{answer.legend?.[key] ?? key}</Typography><Typography variant="body2">{percent(value)}</Typography></Stack><LinearProgress variant="determinate" value={Math.max(0, Math.min(100, value * 100))} /></Box>
                                ))}
                                {typeof answer.confidence === 'number' && <Typography variant="caption" color="text.secondary">{t('decisionTry.confidence')}: {percent(answer.confidence)}</Typography>}
                                {mismatch && <Alert severity="warning" sx={{ mt: 1 }}>{t('decisionTry.differentRule')}</Alert>}
                                <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                                    {[result.data?.selected_provider, result.data?.selected_model, result.data?.latency_ms != null ? formatLatency(result.data.latency_ms) : undefined].filter(Boolean).join(' · ')}
                                </Typography>
                                {result.data?.usage && <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                                    {t('decisionTry.tokens', { input: result.data.usage.input_tokens, output: result.data.usage.output_tokens })}
                                </Typography>}
                                <Box component="details" sx={{ mt: 1 }}><summary>{t('decisionTry.raw')}</summary><Box component="pre" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{JSON.stringify(raw, null, 2)}</Box></Box>
                            </Box>
                        )}
                    </Stack>
                )}
            </AccordionDetails>
        </Accordion>
    );
}
