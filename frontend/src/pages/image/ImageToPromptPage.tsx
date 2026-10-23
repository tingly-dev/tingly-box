import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    CircularProgress,
    IconButton,
    Skeleton,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { IconLibraryPhoto, IconPhotoScan } from '@tabler/icons-react';
import UnifiedCard from '@/components/UnifiedCard';
import { Close, ContentPaste, FileUpload, Palette, tablerMui } from '@/components/icons';
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
    clearImagePromptRuns,
    removeImagePromptRun,
    startImagePromptRun,
    updateImagePromptRun,
    useImagePromptRuns,
    type ImagePromptRun,
} from './prompt/imagePromptStore';
import { DEFAULT_INSTRUCTIONS, INSTRUCTION_STARTERS } from './prompt/imagePromptTemplates';
import InstructionsField from './prompt/InstructionsField';
import PromptRunCard from './prompt/PromptRunCard';
import { useInstructions, useRememberedModel } from './prompt/instructionStarters';
import { PROMPT_TOOLS_SCENARIO as SCENARIO } from './prompt/promptRunStore';
import { takeHandedPicture } from './prompt/pictureHandoff';
import { preparePicture, type Picture } from './prompt/preparePicture';

const PhotoScan = tablerMui(IconPhotoScan);
const LibraryPhoto = tablerMui(IconLibraryPhoto);

const INSTRUCTIONS_KEY = 'image.prompt.instructions';
const MODEL_KEY = 'image.prompt.model';
const WORKBENCH_MIN_HEIGHT = 600;
// The picture's corner buttons sit over any image, light or dark.
const overlayButtonSx = { bgcolor: 'background.paper', '&:hover': { bgcolor: 'background.paper' } } as const;

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
    const [model, chooseModel] = useRememberedModel(MODEL_KEY, models);
    const [instructions, changeInstructions] = useInstructions(INSTRUCTIONS_KEY, DEFAULT_INSTRUCTIONS);

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

                        <InstructionsField
                            value={instructions}
                            onChange={changeInstructions}
                            starters={INSTRUCTION_STARTERS}
                            defaultText={DEFAULT_INSTRUCTIONS}
                            i18nGroup="imagePrompt"
                            onSubmit={getPrompt}
                            canSubmit={canRun}
                        />

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
                                        <ImagePromptRunCard
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

interface ImagePromptRunCardProps {
    run: ImagePromptRun;
    onReuse: (run: ImagePromptRun) => void;
    onUse: (prompt: string) => void;
    onSaveSnippet: (prompt: string) => void;
}

const commitPrompt = (id: string, prompt: string) => updateImagePromptRun(id, { prompt });

// One run: the picture beside its prompt. Memoised: the page re-renders on
// every keystroke in the instructions, and the list keeps runs across days.
const ImagePromptRunCard = memo(({ run, onReuse, onUse, onSaveSnippet }: ImagePromptRunCardProps) => {
    const { t } = useTranslation();
    return (
        <PromptRunCard
            run={run}
            aside={(
                <ButtonBase
                    onClick={() => onReuse(run)}
                    aria-label={t('imagePrompt.reuse', { defaultValue: 'Put this picture and its instructions back in the panel' })}
                    sx={{ flexShrink: 0, width: 96, height: 96, borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover', alignSelf: 'flex-start' }}
                >
                    <ThumbImage src={run.image} alt={run.imageName} edge={THUMB_EDGE_TILE} />
                </ButtonBase>
            )}
            pendingLabel={t('imagePrompt.pending', { defaultValue: 'Looking at the picture…' })}
            failedHint={t('imagePrompt.failedHint', { defaultValue: 'Check that the model accepts images, or choose another vision model.' })}
            onCommit={commitPrompt}
            onUse={onUse}
            onSaveSnippet={onSaveSnippet}
            onRetry={() => { void startImagePromptRun(run, run.id); }}
            onRemove={removeImagePromptRun}
        />
    );
});
ImagePromptRunCard.displayName = 'ImagePromptRunCard';

export default ImageToPromptPage;
