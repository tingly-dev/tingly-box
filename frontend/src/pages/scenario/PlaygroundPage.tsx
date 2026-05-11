import { useCallback, useEffect, useMemo, useState } from 'react';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    CircularProgress,
    FormControl,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    Tab,
    Tabs,
    TextField,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import CardGrid from '@/components/CardGrid';

type Mode = 'chat' | 'image';

interface ChatMessage {
    role: 'user' | 'assistant';
    content: string;
}

const CHAT_SCENARIOS = ['openai', 'anthropic', 'agent'] as const;
const IMAGE_SCENARIO = 'imagegen';

const extractModelsFromRules = (rules: any[] | undefined | null): string[] => {
    if (!Array.isArray(rules)) return [];
    const seen = new Set<string>();
    rules.forEach((r) => {
        if (r?.disabled) return;
        const name = r?.request_model;
        if (typeof name === 'string' && name.trim()) {
            seen.add(name.trim());
        }
    });
    return Array.from(seen);
};

const PlaygroundPage: React.FC = () => {
    const { t } = useTranslation();
    const [mode, setMode] = useState<Mode>('image');

    // Chat state
    const [chatScenario, setChatScenario] = useState<string>('openai');
    const [chatModels, setChatModels] = useState<string[]>([]);
    const [chatModel, setChatModel] = useState<string>('');
    const [chatInput, setChatInput] = useState<string>('');
    const [chatHistory, setChatHistory] = useState<ChatMessage[]>([]);
    const [chatSending, setChatSending] = useState(false);

    // Image state
    const [imageModels, setImageModels] = useState<string[]>([]);
    const [imageModel, setImageModel] = useState<string>('');
    const [imagePrompt, setImagePrompt] = useState<string>('');
    const [imageSize, setImageSize] = useState<string>('1024x1024');
    const [imageCount, setImageCount] = useState<number>(1);
    const [imageResults, setImageResults] = useState<{ url?: string; b64_json?: string }[]>([]);
    const [imageSending, setImageSending] = useState(false);

    const [error, setError] = useState<string>('');
    const [loadingModels, setLoadingModels] = useState(false);

    const loadModels = useCallback(async (scenario: string): Promise<string[]> => {
        const result = await api.getRules(scenario);
        if (!result || result.success === false) {
            return [];
        }
        const rules = Array.isArray(result.data) ? result.data : (Array.isArray(result) ? result : []);
        return extractModelsFromRules(rules);
    }, []);

    // Load image models once
    useEffect(() => {
        let cancelled = false;
        (async () => {
            setLoadingModels(true);
            const models = await loadModels(IMAGE_SCENARIO);
            if (!cancelled) {
                setImageModels(models);
                setImageModel((current) => current || models[0] || '');
                setLoadingModels(false);
            }
        })();
        return () => { cancelled = true; };
    }, [loadModels]);

    // Load chat models when scenario changes
    useEffect(() => {
        let cancelled = false;
        (async () => {
            setLoadingModels(true);
            const models = await loadModels(chatScenario);
            if (!cancelled) {
                setChatModels(models);
                setChatModel((current) => (models.includes(current) ? current : models[0] || ''));
                setLoadingModels(false);
            }
        })();
        return () => { cancelled = true; };
    }, [chatScenario, loadModels]);

    const handleSendChat = useCallback(async () => {
        if (!chatInput.trim() || !chatModel) return;
        const userMsg: ChatMessage = { role: 'user', content: chatInput.trim() };
        const next = [...chatHistory, userMsg];
        setChatHistory(next);
        setChatInput('');
        setChatSending(true);
        setError('');
        try {
            const resp = await api.playgroundChatCompletions(chatScenario, {
                model: chatModel,
                messages: next.map((m) => ({ role: m.role, content: m.content })),
            });
            if (resp?.error) {
                setError(resp.error.message || JSON.stringify(resp.error));
            } else {
                const reply: string = resp?.choices?.[0]?.message?.content ?? '';
                setChatHistory((prev) => [...prev, { role: 'assistant', content: reply }]);
            }
        } catch (err: any) {
            setError(err?.message || 'Request failed');
        } finally {
            setChatSending(false);
        }
    }, [chatInput, chatModel, chatScenario, chatHistory]);

    const handleGenerateImage = useCallback(async () => {
        if (!imagePrompt.trim() || !imageModel) return;
        setImageSending(true);
        setError('');
        setImageResults([]);
        try {
            const resp = await api.playgroundImageGenerate(IMAGE_SCENARIO, {
                model: imageModel,
                prompt: imagePrompt.trim(),
                n: imageCount,
                size: imageSize,
            });
            if (resp?.error) {
                setError(resp.error.message || JSON.stringify(resp.error));
            } else if (Array.isArray(resp?.data)) {
                setImageResults(resp.data);
            } else {
                setError('Unexpected response shape');
            }
        } catch (err: any) {
            setError(err?.message || 'Request failed');
        } finally {
            setImageSending(false);
        }
    }, [imagePrompt, imageModel, imageCount, imageSize]);

    const noChatModels = useMemo(() => chatModels.length === 0, [chatModels]);
    const noImageModels = useMemo(() => imageModels.length === 0, [imageModels]);

    return (
        <PageLayout loading={false}>
            <CardGrid>
                <UnifiedCard
                    size="full"
                    title={t('playground.title', { defaultValue: 'Playground' })}
                >
                    <Tabs
                        value={mode}
                        onChange={(_, v) => { setMode(v); setError(''); }}
                        sx={{ mb: 2 }}
                    >
                        <Tab
                            value="image"
                            label={t('playground.image', { defaultValue: 'Image Generation' })}
                        />
                        <Tab
                            value="chat"
                            label={t('playground.chat', { defaultValue: 'Chat' })}
                        />
                    </Tabs>

                    {error && (
                        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
                            {error}
                        </Alert>
                    )}

                    {mode === 'image' && (
                        <Stack spacing={2}>
                            {noImageModels && !loadingModels && (
                                <Alert severity="info">
                                    {t('playground.noImageModels', {
                                        defaultValue: 'No image generation rules configured. Add one on the Image Gen page first.',
                                    })}
                                </Alert>
                            )}
                            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                                <FormControl size="small" sx={{ minWidth: 240 }}>
                                    <InputLabel id="image-model-label">
                                        {t('playground.model', { defaultValue: 'Model' })}
                                    </InputLabel>
                                    <Select
                                        labelId="image-model-label"
                                        label={t('playground.model', { defaultValue: 'Model' })}
                                        value={imageModel}
                                        onChange={(e) => setImageModel(e.target.value)}
                                        disabled={noImageModels}
                                    >
                                        {imageModels.map((m) => (
                                            <MenuItem key={m} value={m}>{m}</MenuItem>
                                        ))}
                                    </Select>
                                </FormControl>
                                <FormControl size="small" sx={{ minWidth: 140 }}>
                                    <InputLabel id="image-size-label">
                                        {t('playground.size', { defaultValue: 'Size' })}
                                    </InputLabel>
                                    <Select
                                        labelId="image-size-label"
                                        label={t('playground.size', { defaultValue: 'Size' })}
                                        value={imageSize}
                                        onChange={(e) => setImageSize(e.target.value)}
                                    >
                                        <MenuItem value="256x256">256x256</MenuItem>
                                        <MenuItem value="512x512">512x512</MenuItem>
                                        <MenuItem value="1024x1024">1024x1024</MenuItem>
                                        <MenuItem value="1024x1792">1024x1792</MenuItem>
                                        <MenuItem value="1792x1024">1792x1024</MenuItem>
                                    </Select>
                                </FormControl>
                                <TextField
                                    size="small"
                                    type="number"
                                    label={t('playground.count', { defaultValue: 'N' })}
                                    value={imageCount}
                                    onChange={(e) => {
                                        const n = Number(e.target.value);
                                        setImageCount(Number.isFinite(n) && n > 0 ? Math.min(n, 10) : 1);
                                    }}
                                    sx={{ width: 100 }}
                                    inputProps={{ min: 1, max: 10 }}
                                />
                            </Stack>
                            <TextField
                                multiline
                                minRows={4}
                                fullWidth
                                placeholder={t('playground.promptPlaceholder', {
                                    defaultValue: 'Describe the image you want to generate…',
                                })}
                                value={imagePrompt}
                                onChange={(e) => setImagePrompt(e.target.value)}
                                disabled={noImageModels}
                            />
                            <Box>
                                <Button
                                    variant="contained"
                                    onClick={handleGenerateImage}
                                    disabled={imageSending || noImageModels || !imagePrompt.trim() || !imageModel}
                                    startIcon={imageSending ? <CircularProgress size={16} /> : undefined}
                                >
                                    {imageSending
                                        ? t('playground.generating', { defaultValue: 'Generating…' })
                                        : t('playground.generate', { defaultValue: 'Generate' })}
                                </Button>
                            </Box>
                            {imageResults.length > 0 && (
                                <Box
                                    sx={{
                                        display: 'grid',
                                        gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))',
                                        gap: 2,
                                    }}
                                >
                                    {imageResults.map((img, idx) => {
                                        const src = img.url
                                            ? img.url
                                            : img.b64_json
                                                ? `data:image/png;base64,${img.b64_json}`
                                                : '';
                                        return (
                                            <Card key={idx} variant="outlined">
                                                <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
                                                    {src ? (
                                                        <Box
                                                            component="img"
                                                            src={src}
                                                            alt={`result-${idx}`}
                                                            sx={{ width: '100%', display: 'block', borderRadius: 1 }}
                                                        />
                                                    ) : (
                                                        <Typography variant="caption" color="text.secondary">
                                                            empty
                                                        </Typography>
                                                    )}
                                                </CardContent>
                                            </Card>
                                        );
                                    })}
                                </Box>
                            )}
                        </Stack>
                    )}

                    {mode === 'chat' && (
                        <Stack spacing={2}>
                            {noChatModels && !loadingModels && (
                                <Alert severity="info">
                                    {t('playground.noChatModels', {
                                        defaultValue: 'No chat rules configured for this scenario.',
                                    })}
                                </Alert>
                            )}
                            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                                <FormControl size="small" sx={{ minWidth: 160 }}>
                                    <InputLabel id="chat-scenario-label">
                                        {t('playground.scenario', { defaultValue: 'Scenario' })}
                                    </InputLabel>
                                    <Select
                                        labelId="chat-scenario-label"
                                        label={t('playground.scenario', { defaultValue: 'Scenario' })}
                                        value={chatScenario}
                                        onChange={(e) => setChatScenario(e.target.value)}
                                    >
                                        {CHAT_SCENARIOS.map((s) => (
                                            <MenuItem key={s} value={s}>{s}</MenuItem>
                                        ))}
                                    </Select>
                                </FormControl>
                                <FormControl size="small" sx={{ minWidth: 240 }}>
                                    <InputLabel id="chat-model-label">
                                        {t('playground.model', { defaultValue: 'Model' })}
                                    </InputLabel>
                                    <Select
                                        labelId="chat-model-label"
                                        label={t('playground.model', { defaultValue: 'Model' })}
                                        value={chatModel}
                                        onChange={(e) => setChatModel(e.target.value)}
                                        disabled={noChatModels}
                                    >
                                        {chatModels.map((m) => (
                                            <MenuItem key={m} value={m}>{m}</MenuItem>
                                        ))}
                                    </Select>
                                </FormControl>
                                <Button
                                    variant="outlined"
                                    size="small"
                                    onClick={() => setChatHistory([])}
                                    disabled={chatHistory.length === 0}
                                >
                                    {t('playground.clear', { defaultValue: 'Clear' })}
                                </Button>
                            </Stack>

                            <Box
                                sx={{
                                    border: 1,
                                    borderColor: 'divider',
                                    borderRadius: 1,
                                    p: 2,
                                    minHeight: 240,
                                    maxHeight: 480,
                                    overflowY: 'auto',
                                    bgcolor: 'background.default',
                                }}
                            >
                                {chatHistory.length === 0 ? (
                                    <Typography variant="body2" color="text.secondary">
                                        {t('playground.chatEmpty', { defaultValue: 'No messages yet.' })}
                                    </Typography>
                                ) : (
                                    chatHistory.map((m, idx) => (
                                        <Box key={idx} sx={{ mb: 1.5 }}>
                                            <Typography
                                                variant="caption"
                                                color={m.role === 'user' ? 'primary' : 'secondary'}
                                                sx={{ fontWeight: 600 }}
                                            >
                                                {m.role}
                                            </Typography>
                                            <Typography
                                                variant="body2"
                                                sx={{ whiteSpace: 'pre-wrap', mt: 0.25 }}
                                            >
                                                {m.content}
                                            </Typography>
                                        </Box>
                                    ))
                                )}
                            </Box>

                            <TextField
                                multiline
                                minRows={2}
                                fullWidth
                                placeholder={t('playground.chatPlaceholder', {
                                    defaultValue: 'Type a message…',
                                })}
                                value={chatInput}
                                onChange={(e) => setChatInput(e.target.value)}
                                disabled={noChatModels}
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                                        e.preventDefault();
                                        handleSendChat();
                                    }
                                }}
                            />
                            <Box>
                                <Button
                                    variant="contained"
                                    onClick={handleSendChat}
                                    disabled={chatSending || noChatModels || !chatInput.trim() || !chatModel}
                                    startIcon={chatSending ? <CircularProgress size={16} /> : undefined}
                                >
                                    {chatSending
                                        ? t('playground.sending', { defaultValue: 'Sending…' })
                                        : t('playground.send', { defaultValue: 'Send' })}
                                </Button>
                            </Box>
                        </Stack>
                    )}
                </UnifiedCard>
            </CardGrid>
        </PageLayout>
    );
};

export default PlaygroundPage;
