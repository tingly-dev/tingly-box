import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    CircularProgress,
    IconButton,
    ListItemText,
    Menu,
    MenuItem,
    Skeleton,
    Stack,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { IconLibraryPhoto, IconPhotoScan, IconTextPlus } from '@tabler/icons-react';
import UnifiedCard from '@/components/UnifiedCard';
import { CopyIconButton } from '@/components/CopyIconButton';
import { Check, Close, ContentPaste, DeleteOutline, FileUpload, Palette, Refresh, RestartAlt, tablerMui } from '@/components/icons';
import { useNotify } from '@/hooks/useNotify';
import { useRuleManagement } from '@/pages/scenario/hooks/useRuleManagement';
import ImageModelPicker from './components/ImageModelPicker';
import ThumbImage from './components/ThumbImage';
import { THUMB_EDGE_TILE } from './components/imageThumbnails';
import { pastedImageFiles, readClipboardImages } from './components/clipboardImages';
import { imageModelOptions } from './components/imageModelRules';
import AssetPickerDialog from './library/AssetPickerDialog';
import { saveSnippet } from './library/assetStore';
import { deriveLabel } from './profiles/promptLabel';
import {
    IMAGE_PROMPT_SCENARIO as SCENARIO,
    clearImagePromptRuns,
    removeImagePromptRun,
    startImagePromptRun,
    updateImagePromptRun,
    useImagePromptRuns,
    type ImagePromptRun,
} from './prompt/imagePromptStore';
import {
    DEFAULT_INSTRUCTIONS,
    INSTRUCTION_STARTERS,
    STARTERS,
    isDefaultInstructions,
    matchingStarter,
} from './prompt/imagePromptTemplates';
import { takeHandedPicture } from './prompt/pictureHandoff';
import { preparePicture, type Picture } from './prompt/preparePicture';

const PhotoScan = tablerMui(IconPhotoScan);
const TextPlus = tablerMui(IconTextPlus);
const LibraryPhoto = tablerMui(IconLibraryPhoto);

const INSTRUCTIONS_KEY = 'image.prompt.instructions';
const MODEL_KEY = 'image.prompt.model';
const WORKBENCH_MIN_HEIGHT = 600;
// The picture's corner buttons sit over any image, light or dark.
const overlayButtonSx = { bgcolor: 'background.paper', '&:hover': { bgcolor: 'background.paper' } } as const;

const readLocal = (key: string): string | null => {
    try { return localStorage.getItem(key); } catch { return null; }
};
const writeLocal = (key: string, value: string | null) => {
    try {
        if (value === null) localStorage.removeItem(key);
        else localStorage.setItem(key, value);
    } catch { /* kept for this visit only */ }
};

// Image to Prompt (反推提示词): a picture in, an editable prompt out, handed
// on to the Playground or the Library. Like the Playground it has no route
// of its own: the vision model is a provider · model behind an imageprompt
// rule, and every run goes through /tingly/imageprompt. The instructions the
// model gets are the user's to edit; the page starts them from a default.
// See .design/image-to-prompt.md.
const ImageToPromptPage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { notify } = useNotify();
    const showNotification = useCallback(
        (message: string, severity: 'success' | 'info' | 'warning' | 'error') => { notify(severity, message); },
        [notify],
    );

    const { rules, loadingRule, loadRules } = useRuleManagement();
    useEffect(() => { void loadRules(SCENARIO); }, [loadRules]);
    const reloadRules = useCallback(() => loadRules(SCENARIO), [loadRules]);
    const models = useMemo(() => imageModelOptions(rules).map((option) => option.requestModel), [rules]);
    const [selectedModel, setSelectedModel] = useState(() => readLocal(MODEL_KEY) ?? '');
    const model = models.includes(selectedModel) ? selectedModel : (models[0] ?? '');
    const chooseModel = (next: string) => {
        setSelectedModel(next);
        writeLocal(MODEL_KEY, next);
    };

    // Saved only once it differs from the default, so a later, better default
    // still reaches everyone who never touched it.
    const [instructions, setInstructions] = useState(() => readLocal(INSTRUCTIONS_KEY) ?? DEFAULT_INSTRUCTIONS);
    const changeInstructions = (next: string) => {
        setInstructions(next);
        writeLocal(INSTRUCTIONS_KEY, isDefaultInstructions(next) ? null : next);
    };
    const starter = matchingStarter(instructions);
    const isDefault = isDefaultInstructions(instructions);
    const [starterAnchor, setStarterAnchor] = useState<HTMLElement | null>(null);

    const [picture, setPicture] = useState<Picture | null>(null);
    const [preparing, setPreparing] = useState(false);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [libraryOpen, setLibraryOpen] = useState(false);

    // The last picture chosen wins: a slow one still being prepared must not
    // land over one picked after it.
    const pictureRequest = useRef(0);
    const placePicture = useCallback((next: Picture | null) => {
        pictureRequest.current += 1;
        setPreparing(false);
        setPicture(next);
    }, []);
    const loadPicture = useCallback(async (source: File | string, name?: string) => {
        const request = ++pictureRequest.current;
        setPreparing(true);
        try {
            const prepared = await preparePicture(source, name);
            if (request === pictureRequest.current) setPicture(prepared);
        } catch {
            if (request === pictureRequest.current) {
                showNotification(t('imagePrompt.pictureUnreadable', { defaultValue: 'Could not read this picture' }), 'error');
            }
        } finally {
            if (request === pictureRequest.current) setPreparing(false);
        }
    }, [showNotification, t]);

    // Arriving from "Get a prompt from this image" elsewhere: that picture is loaded.
    useEffect(() => {
        const handed = takeHandedPicture();
        if (handed) void loadPicture(handed.src, handed.name);
    }, [loadPicture]);

    const handleFiles = useCallback((files: FileList | File[] | null | undefined) => {
        const image = Array.from(files ?? []).find((file) => file.type.startsWith('image/'));
        if (image) void loadPicture(image);
    }, [loadPicture]);

    const pasteHint = t('playground.pasteHint', { defaultValue: 'Copy an image first, then press Ctrl+V / ⌘V here' });
    const handlePaste = (event: React.ClipboardEvent) => {
        const files = pastedImageFiles(event);
        if (files.length === 0) return;
        event.preventDefault();
        handleFiles(files);
    };
    const pasteFromClipboard = async () => {
        const files = await readClipboardImages();
        if (files.length > 0) handleFiles(files);
        else showNotification(pasteHint, 'info');
    };

    const runs = useImagePromptRuns();
    const canRun = Boolean(picture) && Boolean(model) && !preparing;
    const getPrompt = () => {
        if (!picture || !model) return;
        void startImagePromptRun({ image: picture.dataUrl, imageName: picture.name, model, instructions });
    };

    // Stable, so typing in the instructions does not re-render every card.
    const latest = useRef({ models, changeInstructions, chooseModel });
    latest.current = { models, changeInstructions, chooseModel };
    const reuseRun = useCallback((run: ImagePromptRun) => {
        placePicture({ dataUrl: run.image, name: run.imageName });
        latest.current.changeInstructions(run.instructions);
        if (latest.current.models.includes(run.model)) latest.current.chooseModel(run.model);
    }, [placePicture]);
    const usePrompt = useCallback((prompt: string) => navigate('/image/playground', { state: { prompt } }), [navigate]);
    const saveAsSnippet = useCallback((text: string) => {
        saveSnippet({ name: deriveLabel(text, 12) || t('imagePrompt.title', { defaultValue: 'Image to Prompt' }), text });
        showNotification(t('imagePrompt.savedSnippet', { defaultValue: 'Saved to Library snippets' }), 'success');
    }, [showNotification, t]);

    const sources = [
        { key: 'browse', label: t('playground.referenceBrowse', { defaultValue: 'Browse' }), icon: <FileUpload fontSize="small" />, onClick: () => fileInputRef.current?.click() },
        { key: 'paste', label: t('playground.referencePaste', { defaultValue: 'Paste' }), icon: <ContentPaste fontSize="small" />, onClick: () => { void pasteFromClipboard(); } },
        { key: 'library', label: t('imageLibrary.title', { defaultValue: 'Library' }), icon: <LibraryPhoto fontSize="small" />, onClick: () => setLibraryOpen(true) },
    ];

    return (
        <Box onPaste={handlePaste} sx={{ height: { lg: '100%' }, minHeight: { lg: WORKBENCH_MIN_HEIGHT } }}>
            <UnifiedCard
                size="full"
                titleHeadingLevel={1}
                sx={{ height: { lg: '100%' } }}
                title={t('imagePrompt.title', { defaultValue: 'Image to Prompt' })}
                subtitle={t('imagePrompt.subtitle', { defaultValue: 'Drop in a picture and get a prompt that would make one like it.' })}
                rightAction={(
                    <Button variant="outlined" size="small" startIcon={<Palette />} onClick={() => navigate('/image/playground')}>
                        {t('imageProfile.backToPlayground', { defaultValue: 'Open Playground' })}
                    </Button>
                )}
            >
                <Box
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: { xs: '1fr', lg: 'minmax(360px, 420px) minmax(0, 1fr)' },
                        gridTemplateRows: { lg: 'minmax(0, 1fr)' },
                        height: { lg: '100%' },
                        gap: 3,
                        alignItems: 'stretch',
                    }}
                >
                    <Stack spacing={2} sx={{ minWidth: 0, height: { lg: '100%' } }}>
                        <input
                            ref={fileInputRef}
                            type="file"
                            accept="image/*"
                            hidden
                            onChange={(event) => { handleFiles(event.target.files); event.target.value = ''; }}
                        />
                        <Box>
                            <Typography variant="caption" sx={{ display: 'block', mb: 0.5, color: 'text.secondary' }}>
                                {t('imagePrompt.picture', { defaultValue: 'Picture' })}
                                {' · '}
                                {t('imagePrompt.pictureHint', { defaultValue: 'drop, paste or pick one' })}
                            </Typography>
                            <Box
                                onClick={() => { if (!picture) fileInputRef.current?.click(); }}
                                onDragOver={(event) => event.preventDefault()}
                                onDrop={(event) => { event.preventDefault(); handleFiles(event.dataTransfer.files); }}
                                sx={{
                                    position: 'relative',
                                    height: 220,
                                    border: '1px dashed',
                                    borderColor: 'divider',
                                    borderRadius: 1.5,
                                    bgcolor: 'action.hover',
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    overflow: 'hidden',
                                    cursor: picture ? 'default' : 'pointer',
                                }}
                            >
                                {preparing ? (
                                    <CircularProgress size={24} />
                                ) : picture ? (
                                    <>
                                        <Box component="img" src={picture.dataUrl} alt={picture.name} sx={{ maxWidth: '100%', maxHeight: '100%', objectFit: 'contain' }} />
                                        <Stack direction="row" spacing={0.5} sx={{ position: 'absolute', top: 6, right: 6 }}>
                                            <Tooltip title={t('imagePrompt.replacePicture', { defaultValue: 'Choose another picture' })}>
                                                <IconButton
                                                    size="small"
                                                    onClick={(event) => { event.stopPropagation(); fileInputRef.current?.click(); }}
                                                    aria-label={t('imagePrompt.replacePicture', { defaultValue: 'Choose another picture' })}
                                                    sx={overlayButtonSx}
                                                >
                                                    <FileUpload sx={{ fontSize: 16 }} />
                                                </IconButton>
                                            </Tooltip>
                                            <Tooltip title={t('imagePrompt.removePicture', { defaultValue: 'Remove picture' })}>
                                                <IconButton
                                                    size="small"
                                                    onClick={(event) => { event.stopPropagation(); placePicture(null); }}
                                                    aria-label={t('imagePrompt.removePicture', { defaultValue: 'Remove picture' })}
                                                    sx={overlayButtonSx}
                                                >
                                                    <Close sx={{ fontSize: 16 }} />
                                                </IconButton>
                                            </Tooltip>
                                        </Stack>
                                    </>
                                ) : (
                                    <Stack spacing={1} sx={{ alignItems: 'center' }}>
                                        <PhotoScan sx={{ fontSize: 36, color: 'text.disabled' }} />
                                        <Stack direction="row" spacing={0.5} useFlexGap sx={{ flexWrap: 'wrap', justifyContent: 'center' }}>
                                            {sources.map((source) => (
                                                <Button
                                                    key={source.key}
                                                    size="small"
                                                    color="inherit"
                                                    startIcon={source.icon}
                                                    onClick={(event) => { event.stopPropagation(); source.onClick(); }}
                                                    sx={{ color: 'text.secondary', px: 1.25, '&:hover': { color: 'primary.main' } }}
                                                >
                                                    {source.label}
                                                </Button>
                                            ))}
                                        </Stack>
                                    </Stack>
                                )}
                            </Box>
                        </Box>

                        {loadingRule ? (
                            <Skeleton variant="rounded" height={40} />
                        ) : (
                            <ImageModelPicker
                                scenario={SCENARIO}
                                label={t('imagePrompt.visionModel', { defaultValue: 'Vision model' })}
                                chooserTitle={t('imagePrompt.chooseVisionModel', { defaultValue: 'Choose a model that can see images' })}
                                rules={rules}
                                value={model}
                                onChange={chooseModel}
                                onRulesReload={reloadRules}
                                showNotification={showNotification}
                            />
                        )}

                        {/* The instructions are the one thing here worth tuning,
                            so they are an open field, not a setting behind a
                            dialog. The starters fill it; they are not a mode. */}
                        <Stack spacing={0.5} sx={{ flex: { lg: 1 }, minHeight: 0 }}>
                            <Stack direction="row" sx={{ alignItems: 'center', minHeight: 28 }}>
                                <Typography variant="caption" sx={{ color: 'text.secondary', mr: 'auto' }}>
                                    {t('imagePrompt.instructions', { defaultValue: 'Instructions for the model' })}
                                    {starter === null && ` · ${t('imagePrompt.edited', { defaultValue: 'edited' })}`}
                                </Typography>
                                <Button size="small" onClick={(event) => setStarterAnchor(event.currentTarget)} sx={{ py: 0, minWidth: 0 }}>
                                    {t('imagePrompt.startFrom', { defaultValue: 'Start from…' })}
                                </Button>
                                {!isDefault && (
                                    <Button size="small" color="inherit" startIcon={<RestartAlt sx={{ fontSize: 16 }} />} onClick={() => changeInstructions(DEFAULT_INSTRUCTIONS)} sx={{ py: 0, ml: 0.5, color: 'text.secondary' }}>
                                        {t('imagePrompt.resetInstructions', { defaultValue: 'Default' })}
                                    </Button>
                                )}
                            </Stack>
                            <TextField
                                multiline
                                fullWidth
                                value={instructions}
                                onChange={(event) => changeInstructions(event.target.value)}
                                onKeyDown={(event) => {
                                    if ((event.metaKey || event.ctrlKey) && event.key === 'Enter' && canRun) {
                                        event.preventDefault();
                                        getPrompt();
                                    }
                                }}
                                minRows={8}
                                slotProps={{ htmlInput: { 'aria-label': t('imagePrompt.instructions', { defaultValue: 'Instructions for the model' }) } }}
                                sx={{
                                    flex: { lg: 1 },
                                    minHeight: 0,
                                    display: 'flex',
                                    '& .MuiInputBase-root': { flex: 1, alignItems: 'flex-start', overflow: 'auto', fontSize: 13, lineHeight: 1.55 },
                                }}
                            />
                            <Menu anchorEl={starterAnchor} open={Boolean(starterAnchor)} onClose={() => setStarterAnchor(null)}>
                                {STARTERS.map((key) => (
                                    <MenuItem
                                        key={key}
                                        selected={starter === key}
                                        onClick={() => { setStarterAnchor(null); changeInstructions(INSTRUCTION_STARTERS[key]); }}
                                        sx={{ maxWidth: 360 }}
                                    >
                                        <ListItemText
                                            primary={t(`imagePrompt.starters.${key}`)}
                                            secondary={t(`imagePrompt.starters.${key}Hint`)}
                                            slotProps={{ secondary: { sx: { whiteSpace: 'normal' } } }}
                                        />
                                        {starter === key && <Check sx={{ fontSize: 16, ml: 1, color: 'primary.main' }} />}
                                    </MenuItem>
                                ))}
                            </Menu>
                        </Stack>

                        <Tooltip title={t('imagePrompt.submitShortcut', { defaultValue: '⌘/Ctrl + Enter in the instructions' })} placement="top">
                            <span>
                                <Button variant="contained" fullWidth disabled={!canRun} onClick={getPrompt} startIcon={<PhotoScan />}>
                                    {t('imagePrompt.run', { defaultValue: 'Get prompt' })}
                                </Button>
                            </span>
                        </Tooltip>
                    </Stack>

                    <Box
                        sx={{
                            minWidth: 0,
                            height: { lg: '100%' },
                            display: 'flex',
                            flexDirection: 'column',
                            border: 1,
                            borderColor: 'divider',
                            borderRadius: 2,
                            overflow: 'hidden',
                        }}
                    >
                        <Stack direction="row" sx={{ alignItems: 'center', px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}>
                            <Typography variant="subtitle2" sx={{ mr: 'auto' }}>
                                {t('imagePrompt.results', { defaultValue: 'Prompts' })}
                            </Typography>
                            {runs.length > 0 && (
                                <Button size="small" color="inherit" onClick={clearImagePromptRuns} sx={{ color: 'text.secondary' }}>
                                    {t('imagePrompt.clear', { defaultValue: 'Clear' })}
                                </Button>
                            )}
                        </Stack>
                        <Box sx={{ flex: 1, minHeight: { xs: 240, lg: 0 }, overflow: 'auto', p: 2 }}>
                            {runs.length === 0 ? (
                                <Stack sx={{ height: '100%', minHeight: 200, alignItems: 'center', justifyContent: 'center', textAlign: 'center', color: 'text.secondary', px: 4 }}>
                                    <Typography variant="body2">
                                        {t('imagePrompt.empty', { defaultValue: 'Prompts you get appear here.' })}
                                    </Typography>
                                    <Typography variant="caption" sx={{ mt: 0.5, color: 'text.disabled', maxWidth: 420 }}>
                                        {t('imagePrompt.emptyHint', { defaultValue: 'Edit one, then use it in the Playground or keep it as a snippet. “Get a prompt” on any image in the Playground opens it here too.' })}
                                    </Typography>
                                </Stack>
                            ) : (
                                <Stack spacing={1.5}>
                                    {runs.map((run) => (
                                        <PromptRunCard
                                            key={run.id}
                                            run={run}
                                            onReuse={reuseRun}
                                            onUse={usePrompt}
                                            onSaveSnippet={saveAsSnippet}
                                        />
                                    ))}
                                </Stack>
                            )}
                        </Box>
                    </Box>
                </Box>
            </UnifiedCard>
            <AssetPickerDialog
                open={libraryOpen}
                remaining={1}
                onClose={() => setLibraryOpen(false)}
                onPick={(assets) => {
                    setLibraryOpen(false);
                    if (assets[0]) void loadPicture(assets[0].src, assets[0].name);
                }}
            />
        </Box>
    );
};

interface PromptRunCardProps {
    run: ImagePromptRun;
    onReuse: (run: ImagePromptRun) => void;
    onUse: (prompt: string) => void;
    onSaveSnippet: (prompt: string) => void;
}

// Runs are kept across days: anything not from today says which day.
const runTimeLabel = (createdAt: number): string => {
    const created = new Date(createdAt);
    return created.toDateString() === new Date().toDateString()
        ? created.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
        : created.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
};

// One run: the picture, and the prompt as an editable draft with the ways
// to hand it on. Memoised: the page re-renders on every keystroke in the
// instructions, and the list keeps runs across days.
const PromptRunCard = memo(({ run, onReuse, onUse, onSaveSnippet }: PromptRunCardProps) => {
    const { t } = useTranslation();
    const [draft, setDraft] = useState(run.prompt);
    useEffect(() => { setDraft(run.prompt); }, [run.prompt]);
    const commit = () => { if (draft !== run.prompt) updateImagePromptRun(run.id, { prompt: draft }); };
    const time = runTimeLabel(run.createdAt);

    return (
        <Box sx={{ display: 'flex', gap: 1.5, p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1.5, bgcolor: 'background.paper' }}>
            <ButtonBase
                onClick={() => onReuse(run)}
                aria-label={t('imagePrompt.reuse', { defaultValue: 'Put this picture and its instructions back in the panel' })}
                sx={{ flexShrink: 0, width: 96, height: 96, borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover', alignSelf: 'flex-start' }}
            >
                <ThumbImage src={run.image} alt={run.imageName} edge={THUMB_EDGE_TILE} />
            </ButtonBase>
            <Stack spacing={1} sx={{ flex: 1, minWidth: 0 }}>
                {run.status === 'pending' ? (
                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minHeight: 72, color: 'text.secondary' }}>
                        <CircularProgress size={16} />
                        <Typography variant="body2">{t('imagePrompt.pending', { defaultValue: 'Looking at the picture…' })}</Typography>
                    </Stack>
                ) : run.status === 'failed' ? (
                    <Box sx={{ minHeight: 72 }}>
                        <Typography variant="body2" color="error" sx={{ wordBreak: 'break-word' }}>{run.error}</Typography>
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                            {t('imagePrompt.failedHint', { defaultValue: 'Check that the model accepts images, or choose another vision model.' })}
                        </Typography>
                    </Box>
                ) : (
                    <TextField
                        multiline
                        fullWidth
                        size="small"
                        value={draft}
                        onChange={(event) => setDraft(event.target.value)}
                        onBlur={commit}
                        slotProps={{ htmlInput: { 'aria-label': t('imagePrompt.promptField', { defaultValue: 'Prompt' }) } }}
                        sx={{ '& .MuiInputBase-root': { fontSize: 13, lineHeight: 1.55 } }}
                    />
                )}
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
                    <Typography variant="caption" sx={{ color: 'text.secondary', mr: 'auto', minWidth: 0 }} noWrap>
                        {run.model} · {time}
                    </Typography>
                    {run.status === 'completed' && (
                        <>
                            <CopyIconButton
                                value={draft}
                                size="small"
                                iconSize={16}
                                label={t('playground.copyPrompt', { defaultValue: 'Copy prompt' })}
                                copiedLabel={t('common.copied', { defaultValue: 'Copied' })}
                            />
                            <Tooltip title={t('imagePrompt.saveSnippet', { defaultValue: 'Keep as a snippet' })}>
                                <IconButton size="small" onClick={() => onSaveSnippet(draft)} aria-label={t('imagePrompt.saveSnippet', { defaultValue: 'Keep as a snippet' })}>
                                    <TextPlus sx={{ fontSize: 16 }} />
                                </IconButton>
                            </Tooltip>
                            <Button size="small" variant="outlined" startIcon={<Palette sx={{ fontSize: 16 }} />} onClick={() => onUse(draft)} sx={{ ml: 0.5 }}>
                                {t('imagePrompt.usePlayground', { defaultValue: 'Use in Playground' })}
                            </Button>
                        </>
                    )}
                    {run.status === 'failed' && (
                        <Button size="small" startIcon={<Refresh sx={{ fontSize: 16 }} />} onClick={() => { void startImagePromptRun(run, run.id); }}>
                            {t('imagePrompt.retry', { defaultValue: 'Retry' })}
                        </Button>
                    )}
                    <Tooltip title={t('imagePrompt.remove', { defaultValue: 'Remove' })}>
                        <IconButton size="small" onClick={() => removeImagePromptRun(run.id)} aria-label={t('imagePrompt.remove', { defaultValue: 'Remove' })}>
                            <DeleteOutline sx={{ fontSize: 16 }} />
                        </IconButton>
                    </Tooltip>
                </Stack>
            </Stack>
        </Box>
    );
});
PromptRunCard.displayName = 'PromptRunCard';

export default ImageToPromptPage;
