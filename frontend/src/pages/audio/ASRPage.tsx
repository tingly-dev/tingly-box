import { useEffect, useRef, useState } from 'react';
import { Alert, Autocomplete, Button, Card, CardContent, Checkbox, FormControlLabel, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { TranscriptionCreateParamsNonStreaming } from 'openai/resources/audio/transcriptions';
import { getOpenAIClient } from '@/services/modelApi';
import AudioApiPage from './AudioApiPage';
import { MAX_AUDIO_UPLOAD_BYTES, useAudioRecorder } from './useAudioRecorder';

type Format = 'json' | 'verbose_json' | 'text' | 'srt' | 'vtt';
const TranscriptionPlayground = ({ models }: { models: string[] }) => {
    const { t } = useTranslation();
    const [model, setModel] = useState('');
    const [file, setFile] = useState<File | null>(null);
    const [task, setTask] = useState('transcribe');
    const [language, setLanguage] = useState('');
    const [prompt, setPrompt] = useState('');
    const [format, setFormat] = useState<Format>('json');
    const [timestamps, setTimestamps] = useState(false);
    const [streaming, setStreaming] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [output, setOutput] = useState('');
    const [download, setDownload] = useState<{ url: string; extension: string } | null>(null);
    const controller = useRef<AbortController | null>(null);
    const recording = useAudioRecorder(setFile);
    useEffect(() => () => controller.current?.abort(), []);
    useEffect(() => () => { if (download) URL.revokeObjectURL(download.url); }, [download]);
    const selectedModel = model || models[0] || '';
    const fileTooLarge = !!file && file.size > MAX_AUDIO_UPLOAD_BYTES;
    const run = async () => {
        if (!file || !file.size || fileTooLarge) return;
        const request = new AbortController();
        controller.current = request;
        setBusy(true); setError(''); setOutput(''); setDownload(null);
        try {
            const client = await getOpenAIClient('asr');
            if (request.signal.aborted) return;
            let raw: string;
            let extension: string;
            if (streaming && task === 'transcribe') {
                const events = await client.audio.transcriptions.create({
                    model: selectedModel, file, stream: true, ...(language.trim() ? { language: language.trim() } : {}), ...(prompt ? { prompt } : {}),
                }, { signal: request.signal });
                let text = '';
                let completed = false;
                for await (const event of events) {
                    if (request.signal.aborted) return;
                    if (event.type === 'transcript.text.delta') text += event.delta;
                    if (event.type === 'transcript.text.done') { text = event.text; completed = true; }
                    setOutput(text);
                }
                if (!completed) throw new Error(t('audio.incompleteTranscript'));
                raw = text; extension = 'txt';
            } else {
                const options = { signal: request.signal };
                const params: TranscriptionCreateParamsNonStreaming = {
                    model: selectedModel, file, response_format: format,
                    ...(prompt ? { prompt } : {}),
                    ...(task === 'transcribe' && language.trim() ? { language: language.trim() } : {}),
                    ...(task === 'transcribe' && timestamps && format === 'verbose_json' ? { timestamp_granularities: ['word', 'segment'] } : {}),
                };
                const response = await (task === 'translate'
                    ? client.audio.translations.create({ model: selectedModel, file, response_format: format, ...(prompt ? { prompt } : {}) }, options)
                    : client.audio.transcriptions.create(params, options)).asResponse();
                raw = await response.text();
                extension = format.includes('json') ? 'json' : format === 'text' ? 'txt' : format;
                if (format.includes('json')) raw = JSON.stringify(JSON.parse(raw), null, 2);
            }
            if (request.signal.aborted) return;
            if (!raw.trim()) throw new Error(t('audio.emptyResponse'));
            setOutput(raw);
            setDownload({ url: URL.createObjectURL(new Blob([raw], { type: extension === 'json' ? 'application/json' : 'text/plain;charset=utf-8' })), extension });
        } catch (cause) {
            if (!request.signal.aborted) setError(cause instanceof Error ? cause.message : String(cause));
        } finally {
            if (!request.signal.aborted) setBusy(false);
        }
    };
    const disabled = busy || recording.recording || recording.starting;
    return (
        <Card><CardContent><Stack spacing={2}>
            <Typography variant="h6">{t('audio.asrPlayground')}</Typography>
            <Typography color="text.secondary">{t('audio.asrHint')}</Typography>
            <Autocomplete freeSolo options={models} inputValue={selectedModel} disabled={disabled}
                onInputChange={(_, value) => setModel(value)} renderInput={params => <TextField {...params} label={t('audio.model')} />} />
            {!models.length && <Alert severity="info">{t('audio.configureModel')}</Alert>}
            <Stack direction="row" spacing={1}>
                <Button component="label" variant="outlined" disabled={disabled}>{t('audio.upload')}
                    <input hidden type="file" accept="audio/*,video/mp4,video/webm" aria-label={t('audio.upload')} disabled={disabled}
                        onChange={e => setFile(e.target.files?.[0] ?? null)} />
                </Button>
                {recording.recording
                    ? <Button color="error" onClick={recording.stop}>{t('audio.stopRecording')}</Button>
                    : <Button onClick={recording.start} disabled={disabled}>{t('audio.record')}</Button>}
            </Stack>
            {recording.recording && <Alert severity="info">{t('audio.recordingHint')}</Alert>}
            {recording.error && <Alert severity="error">{recording.error}</Alert>}
            {file && <Typography>{file.name} · {(file.size / 1024 / 1024).toFixed(2)} MiB</Typography>}
            {fileTooLarge && <Alert severity="error">{t('audio.fileTooLarge')}</Alert>}
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                <TextField label={t('audio.task')} value={task} select disabled={disabled} fullWidth
                    onChange={e => { setTask(e.target.value); if (e.target.value === 'translate') setStreaming(false); }}>
                    <MenuItem value="transcribe">{t('audio.transcribe')}</MenuItem><MenuItem value="translate">{t('audio.translate')}</MenuItem>
                </TextField>
                <TextField label={t('audio.language')} value={language} onChange={e => setLanguage(e.target.value)} disabled={disabled || task === 'translate'} fullWidth />
                <TextField label={t('audio.format')} value={format} onChange={e => setFormat(e.target.value as Format)} select disabled={disabled || streaming} fullWidth>
                    {(['json', 'verbose_json', 'text', 'srt', 'vtt'] as const).map(value => <MenuItem key={value} value={value}>{value}</MenuItem>)}
                </TextField>
            </Stack>
            <TextField label={t('audio.prompt')} value={prompt} onChange={e => setPrompt(e.target.value)} disabled={disabled} />
            <FormControlLabel label={t('audio.timestamps')} control={<Checkbox checked={timestamps} onChange={(_, value) => setTimestamps(value)} disabled={disabled || task === 'translate' || format !== 'verbose_json' || streaming} />} />
            <FormControlLabel label={t('audio.streaming')} control={<Checkbox checked={streaming} onChange={(_, value) => { setStreaming(value); if (value) setFormat('json'); }} disabled={disabled || task === 'translate'} />} />
            <Stack direction="row" spacing={1}>
                <Button variant="contained" onClick={run} disabled={disabled || !selectedModel.trim() || !file?.size || fileTooLarge}>{t('audio.recognize')}</Button>
                {busy && <Button onClick={() => { controller.current?.abort(); setBusy(false); }}>{t('audio.cancel')}</Button>}
            </Stack>
            {error && <Alert severity="error">{error}</Alert>}
            {output && <TextField label={t('audio.transcript')} value={output} multiline minRows={5} slotProps={{ input: { readOnly: true } }} />}
            {download && <Button component="a" href={download.url} download={`transcript.${download.extension}`}>{t('audio.downloadTranscript')}</Button>}
        </Stack></CardContent></Card>
    );
};

export default function ASRPage() {
    const { t } = useTranslation();
    return <AudioApiPage scenario="asr" title={t('audio.asrTitle')}>{models => <TranscriptionPlayground models={models} />}</AudioApiPage>;
}
