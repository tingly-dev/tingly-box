import { useEffect, useRef, useState } from 'react';
import { Alert, Autocomplete, Button, Card, CardContent, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { SpeechCreateParams } from 'openai/resources/audio/speech';
import { getOpenAIClient } from '@/services/modelApi';
import AudioApiPage from './AudioApiPage';

const SpeechPlayground = ({ models }: { models: string[] }) => {
    const { t } = useTranslation();
    const [model, setModel] = useState('');
    const [input, setInput] = useState('');
    const [voice, setVoice] = useState('alloy');
    const [speed, setSpeed] = useState('1');
    const [format, setFormat] = useState<NonNullable<SpeechCreateParams['response_format']>>('mp3');
    const [instructions, setInstructions] = useState('');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [result, setResult] = useState<{ url: string; format: string } | null>(null);
    const controller = useRef<AbortController | null>(null);
    const resultURL = useRef<string | null>(null);
    useEffect(() => () => {
        controller.current?.abort();
        if (resultURL.current) URL.revokeObjectURL(resultURL.current);
    }, []);
    const selectedModel = model || models[0] || '';
    const validSpeed = Number.isFinite(Number(speed)) && Number(speed) >= 0.25 && Number(speed) <= 4;
    const run = async () => {
        const request = new AbortController();
        controller.current = request;
        setBusy(true); setError(''); setResult(null);
        if (resultURL.current) { URL.revokeObjectURL(resultURL.current); resultURL.current = null; }
        try {
            const client = await getOpenAIClient('tts');
            if (request.signal.aborted) return;
            const response = await client.audio.speech.create({
                model: selectedModel, input, voice: voice as SpeechCreateParams['voice'], speed: Number(speed),
                response_format: format, ...(instructions ? { instructions } : {}),
            }, { signal: request.signal });
            const blob = await response.blob();
            if (request.signal.aborted) return;
            if (!blob.size) throw new Error(t('audio.emptyResponse'));
            const url = URL.createObjectURL(blob);
            resultURL.current = url;
            setResult({ url, format });
        } catch (cause) {
            if (!request.signal.aborted) setError(cause instanceof Error ? cause.message : String(cause));
        } finally {
            if (!request.signal.aborted) setBusy(false);
        }
    };
    return (
        <Card><CardContent><Stack spacing={2}>
            <Typography variant="h6">{t('audio.ttsPlayground')}</Typography>
            <Typography color="text.secondary">{t('audio.ttsHint')}</Typography>
            <Autocomplete freeSolo options={models} inputValue={selectedModel} disabled={busy}
                onInputChange={(_, value) => setModel(value)} renderInput={params => <TextField {...params} label={t('audio.model')} />} />
            {!models.length && <Alert severity="info">{t('audio.configureModel')}</Alert>}
            <TextField label={t('audio.input')} value={input} onChange={e => setInput(e.target.value)} multiline minRows={4} disabled={busy} />
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                <TextField label={t('audio.voice')} value={voice} onChange={e => setVoice(e.target.value)} disabled={busy} fullWidth />
                <TextField label={t('audio.speed')} type="number" value={speed} onChange={e => setSpeed(e.target.value)}
                    slotProps={{ htmlInput: { min: 0.25, max: 4, step: 0.05 } }} error={!validSpeed} disabled={busy} fullWidth />
                <TextField label={t('audio.format')} value={format} onChange={e => setFormat(e.target.value as typeof format)} select disabled={busy} fullWidth>
                    {(['mp3', 'opus', 'aac', 'flac', 'wav', 'pcm'] as const).map(value => <MenuItem key={value} value={value}>{value}</MenuItem>)}
                </TextField>
            </Stack>
            <TextField label={t('audio.instructions')} value={instructions} onChange={e => setInstructions(e.target.value)} disabled={busy} />
            <Stack direction="row" spacing={1}>
                <Button variant="contained" onClick={run} disabled={busy || !selectedModel.trim() || !input.trim() || !voice.trim() || !validSpeed}>{t('audio.generate')}</Button>
                {busy && <Button onClick={() => { controller.current?.abort(); setBusy(false); }}>{t('audio.cancel')}</Button>}
            </Stack>
            {error && <Alert severity="error">{error}</Alert>}
            {result && <Stack spacing={1}>
                {result.format !== 'pcm' && <audio controls src={result.url} aria-label={t('audio.preview')} />}
                <Button component="a" href={result.url} download={`speech.${result.format}`}>{t('audio.download')}</Button>
            </Stack>}
        </Stack></CardContent></Card>
    );
};

export default function TTSPage() {
    const { t } = useTranslation();
    return <AudioApiPage scenario="tts" title={t('audio.ttsTitle')}>{models => <SpeechPlayground models={models} />}</AudioApiPage>;
}
