import {Check, CheckCircle, Close, Help} from '@tingly/ui/icons';
import {Alert, Box, Button, Chip, Paper, Stack, TextField, Typography} from '@mui/material';
import {alpha} from '@mui/material/styles';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import {useDraft} from './context';
import type {AskOption, AskQuestion} from './model';

// QuestionDraft is a question card's state: what is picked and typed for
// each question (by index), and whether the answer went out.
interface QuestionDraft {
    picked: string[][];
    typed: string[];
    status: 'idle' | 'sending' | 'accepted';
}
const decodeQuestion = (value: unknown): QuestionDraft => {
    const saved = value && typeof value === 'object' ? value as Record<string, unknown> : {};
    const strings = (v: unknown) => Array.isArray(v) ? v.map((s) => typeof s === 'string' ? s : '') : [];
    return {
        picked: Array.isArray(saved.picked) ? saved.picked.map(strings) : [],
        typed: strings(saved.typed),
        status: saved.status === 'accepted' ? 'accepted' : 'idle',
    };
};

// answerOf is one question's answer: the typed words win for a single
// choice; for several they are added to the picked labels.
const answerOf = (q: AskQuestion, picked: string[] = [], typed = ''): string => {
    const words = typed.trim();
    if (!q.multiSelect) return words || picked[0] || '';
    return [...picked, ...(words ? [words] : [])].join(', ');
};

const OptionButton = ({option, selected, multi, disabled, onClick}: {
    option: AskOption; selected: boolean; multi: boolean; disabled: boolean; onClick: () => void;
}) => {
    const {t} = useTranslation();
    return (
        <Box
            component="button"
            type="button"
            role={multi ? 'checkbox' : 'radio'}
            aria-checked={selected}
            disabled={disabled}
            onClick={onClick}
            sx={{
                display: 'flex', alignItems: 'flex-start', gap: 1, width: '100%', textAlign: 'left',
                px: 1.25, py: 0.875, borderRadius: 1, border: 1, font: 'inherit', cursor: disabled ? 'default' : 'pointer',
                borderColor: selected ? 'primary.main' : 'divider',
                bgcolor: (theme) => selected ? alpha(theme.palette.primary.main, 0.08) : 'transparent',
                color: 'text.primary',
                '&:hover:not(:disabled)': {borderColor: 'primary.main'},
                '&:focus-visible': {outline: 2, outlineColor: 'primary.main', outlineOffset: 1},
            }}
        >
            <Box sx={{flex: 1, minWidth: 0}}>
                <Stack direction="row" spacing={0.75} sx={{alignItems: 'center', minWidth: 0}}>
                    <Typography variant="body2" sx={{fontWeight: 600, color: 'inherit', fontSize: '0.875rem', wordBreak: 'break-word'}}>{option.text}</Typography>
                    {option.recommended && (
                        <Chip size="small" color="primary" variant="outlined" label={t('projects.transcript.recommended', {defaultValue: 'Recommended'})} sx={{height: 18, fontSize: '0.6875rem'}}/>
                    )}
                </Stack>
                {option.description && (
                    <Typography variant="body2" sx={{color: 'text.secondary', mt: 0.25, wordBreak: 'break-word'}}>{option.description}</Typography>
                )}
            </Box>
            {selected && <CheckCircle sx={{fontSize: 18, color: 'primary.main', mt: 0.125, flexShrink: 0}}/>}
        </Box>
    );
};

// QuestionCard is a question the live turn waits on, as a choice: each
// question's options as buttons (the suggested one marked), a box for
// other words, and one send. A single question with a single choice goes
// out on the click itself.
const QuestionCard = ({requestId, title, questions, onRespond}: {
    requestId: string;
    // The question as the engine summarized it, used when there are no
    // structured questions to show.
    title: string;
    questions: AskQuestion[];
    onRespond: (approved: boolean, answer: string, remember?: boolean, answers?: Record<string, string>) => Promise<boolean>;
}) => {
    const {t} = useTranslation();
    const [draft, setDraft, latest] = useDraft(`questionDraft:${requestId}`, decodeQuestion);
    const [failed, setFailed] = useState(false);
    const busy = draft.status === 'sending';
    const submitted = draft.status === 'accepted';
    const locked = busy || submitted;
    const shown = questions.length > 0 ? questions : [{question: title, options: [], multiSelect: false}];
    const oneTap = shown.length === 1 && !shown[0].multiSelect && shown[0].options.length > 0;

    const answersOf = (d: QuestionDraft) => {
        const out: Record<string, string> = {};
        shown.forEach((q, i) => {
            const a = answerOf(q, d.picked[i], d.typed[i]);
            if (a) out[q.question] = a;
        });
        return out;
    };
    const complete = Object.keys(answersOf(draft)).length === shown.length;

    const send = async (approved: boolean, d: QuestionDraft) => {
        if (latest().status !== 'idle') return;
        const answers = approved ? answersOf(d) : {};
        if (approved && Object.keys(answers).length < shown.length) return;
        setDraft({...d, status: 'sending'});
        setFailed(false);
        try {
            if (await onRespond(approved, '', undefined, answers)) setDraft((p) => ({...p, status: 'accepted'}));
            else setFailed(true);
        } catch {
            setFailed(true);
        } finally {
            setDraft((p) => p.status === 'sending' ? {...p, status: 'idle'} : p);
        }
    };

    const pick = (qi: number, label: string) => {
        const q = shown[qi];
        const d = latest();
        const picked = [...d.picked];
        const current = picked[qi] ?? [];
        picked[qi] = q.multiSelect
            ? (current.includes(label) ? current.filter((l) => l !== label) : [...current, label])
            : [label];
        // A pick replaces words typed for a single choice.
        const typed = [...d.typed];
        if (!q.multiSelect) typed[qi] = '';
        const next = {...d, picked, typed};
        if (oneTap) void send(true, next);
        else setDraft(next);
    };
    const type = (qi: number, text: string) => setDraft((p) => {
        const typed = [...p.typed];
        typed[qi] = text;
        // Words replace the pick of a single choice.
        const picked = [...p.picked];
        if (!shown[qi].multiSelect && text.trim()) picked[qi] = [];
        return {...p, typed, picked};
    });

    return (
        <Paper data-request-id={requestId} variant="outlined" sx={{p: 1.5, borderRadius: 1.5, borderColor: 'primary.main'}}>
            <Stack spacing={1.75}>
                {shown.map((q, qi) => (
                    <Box key={qi} role="group" aria-label={q.question}>
                        <Stack direction="row" spacing={1} sx={{alignItems: 'flex-start', mb: 1}}>
                            <Help sx={{fontSize: 16, color: 'primary.main', mt: 0.25, flexShrink: 0}}/>
                            <Box sx={{minWidth: 0}}>
                                {q.header && <Typography variant="caption" sx={{color: 'text.secondary', display: 'block'}}>{q.header}</Typography>}
                                <Typography variant="body2" sx={{fontWeight: 600, color: 'text.primary', fontSize: '0.875rem', wordBreak: 'break-word'}}>{q.question}</Typography>
                                {q.multiSelect && (
                                    <Typography variant="caption" sx={{color: 'text.secondary'}}>{t('projects.transcript.pickSeveral', {defaultValue: 'Pick any that apply'})}</Typography>
                                )}
                            </Box>
                        </Stack>
                        {q.options.length > 0 && (
                            <Stack spacing={0.75} role={q.multiSelect ? 'group' : 'radiogroup'} sx={{mb: 1}}>
                                {q.options.map((o) => (
                                    <OptionButton
                                        key={o.label}
                                        option={o}
                                        multi={q.multiSelect}
                                        selected={(draft.picked[qi] ?? []).includes(o.label)}
                                        disabled={locked}
                                        onClick={() => pick(qi, o.label)}
                                    />
                                ))}
                            </Stack>
                        )}
                        <TextField
                            size="small"
                            fullWidth
                            multiline
                            maxRows={6}
                            disabled={locked}
                            value={draft.typed[qi] ?? ''}
                            onChange={(e) => type(qi, e.target.value)}
                            onKeyDown={(event) => {
                                if (event.key === 'Enter' && (event.ctrlKey || event.metaKey) && !event.nativeEvent.isComposing) {
                                    event.preventDefault();
                                    void send(true, latest());
                                }
                            }}
                            placeholder={q.options.length > 0
                                ? t('projects.transcript.answerOther', {defaultValue: 'Or answer in your own words'})
                                : t('projects.transcript.answerPlaceholder', {defaultValue: 'Your answer'})}
                            slotProps={{htmlInput: {'aria-label': q.options.length > 0
                                ? t('projects.transcript.answerOther', {defaultValue: 'Or answer in your own words'})
                                : t('projects.transcript.answerPlaceholder', {defaultValue: 'Your answer'})}}}
                        />
                    </Box>
                ))}
                {failed && <Alert severity="error">{t('projects.transcript.responseRetry', {defaultValue: 'Response was not accepted. Your answer is kept; try again.'})}</Alert>}
                {submitted && <Typography role="status" variant="caption">{t('projects.transcript.responseSubmitted', {defaultValue: 'Response sent — waiting for the agent…'})}</Typography>}
                <Stack direction="row" spacing={1}>
                    <Button size="small" variant="contained" startIcon={<Check/>} disabled={locked || !complete} onClick={() => void send(true, latest())}>
                        {t('projects.transcript.answer', {defaultValue: 'Answer'})}
                    </Button>
                    <Button size="small" variant="outlined" color="inherit" startIcon={<Close/>} disabled={locked} onClick={() => void send(false, latest())}>
                        {t('projects.transcript.decline', {defaultValue: 'Decline'})}
                    </Button>
                </Stack>
            </Stack>
        </Paper>
    );
};

export default QuestionCard;
