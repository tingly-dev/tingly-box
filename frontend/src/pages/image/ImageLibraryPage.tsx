import { useEffect, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import UnifiedCard from '@/components/UnifiedCard';
import { CopyIconButton } from '@/components/CopyIconButton';
import { Add, Close, DeleteOutline, Download, Edit } from '@/components/icons';
import { api } from '@/services/api';
import { fontMono } from '@/theme/fonts';
import { downloadImage } from '@/utils/download';
import type { ImageAsset, PromptSnippet } from './library/assetTypes';
import { removeAsset, removeSnippet, saveSnippet, useAssets, useSnippets } from './library/assetStore';

type Tab = 'images' | 'snippets';

// The library (素材库): what the user chose to keep — images and prompt
// snippets. History is what happened and can be cleared; this is what stays.
// A collection only: it does not track which profiles use an image (using one
// copies it). See .design/image-library.md.
const ImageLibraryPage: React.FC = () => {
    const { t } = useTranslation();
    const [params, setParams] = useSearchParams();
    const tab: Tab = params.get('tab') === 'snippets' ? 'snippets' : 'images';
    const assets = useAssets();
    const snippets = useSnippets();
    const [openAsset, setOpenAsset] = useState<ImageAsset | null>(null);
    const [confirmingDelete, setConfirmingDelete] = useState(false);
    const [snippetDraft, setSnippetDraft] = useState<Partial<PromptSnippet> | null>(null);

    // Where the library lives on disk — the same folder generated images
    // already land in.
    const [outputDir, setOutputDir] = useState('');
    useEffect(() => {
        let cancelled = false;
        void api.getImageGenInfo().then((result) => {
            if (!cancelled && result?.success) setOutputDir(result.output_dir ?? '');
        });
        return () => { cancelled = true; };
    }, []);

    const deleteAsset = (asset: ImageAsset) => {
        removeAsset(asset.id);
        setOpenAsset(null);
        setConfirmingDelete(false);
    };

    return (
        <>
            <UnifiedCard
                size="full"
                titleHeadingLevel={1}
                title={t('imageLibrary.title', { defaultValue: 'Library' })}
                subtitle={outputDir ? (
                    <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
                        <Box component="span">{t('imageLibrary.storedIn', { defaultValue: 'Stored in' })}:</Box>
                        <Box component="span" sx={{ fontFamily: fontMono, wordBreak: 'break-all' }}>{outputDir}</Box>
                        <CopyIconButton
                            value={outputDir}
                            label={t('common.copy', { defaultValue: 'Copy' })}
                            copiedLabel={t('common.copied', { defaultValue: 'Copied!' })}
                            iconSize={14}
                            sx={{ p: 0.25 }}
                        />
                    </Stack>
                ) : undefined}
            >
                <Stack direction="row" sx={{ mb: 2.5, alignItems: 'center' }}>
                    <ToggleButtonGroup
                        size="small"
                        exclusive
                        value={tab}
                        onChange={(_, value: Tab | null) => { if (value) setParams(value === 'images' ? {} : { tab: value }, { replace: true }); }}
                    >
                        <ToggleButton value="images" sx={{ px: 1.5 }}>
                            {t('imageLibrary.images', { defaultValue: 'Images' })} · {assets.length}
                        </ToggleButton>
                        <ToggleButton value="snippets" sx={{ px: 1.5 }}>
                            {t('imageLibrary.snippets', { defaultValue: 'Snippets' })} · {snippets.length}
                        </ToggleButton>
                    </ToggleButtonGroup>
                    {tab === 'snippets' && (
                        <Button startIcon={<Add />} onClick={() => setSnippetDraft({ name: '', text: '' })} sx={{ ml: 'auto' }}>
                            {t('imageLibrary.newSnippet', { defaultValue: 'New snippet' })}
                        </Button>
                    )}
                </Stack>

                {tab === 'images' && (
                    assets.length === 0 ? (
                        <Typography sx={{ color: 'text.secondary', fontSize: 14, py: 6, textAlign: 'center' }}>
                            {t('imageLibrary.emptyImages', { defaultValue: 'Nothing kept yet. Open any image in the Playground and choose “Keep in library”.' })}
                        </Typography>
                    ) : (
                        <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(168px, 1fr))', gap: 2 }}>
                            {assets.map((asset) => (
                                <ButtonBase
                                    key={asset.id}
                                    onClick={() => { setConfirmingDelete(false); setOpenAsset(asset); }}
                                    sx={{ display: 'flex', flexDirection: 'column', alignItems: 'stretch', textAlign: 'left', borderRadius: 1.5 }}
                                >
                                    <Box sx={{ aspectRatio: '1 / 1', borderRadius: 1.5, overflow: 'hidden', bgcolor: 'action.hover' }}>
                                        <Box component="img" src={asset.src} alt={asset.name} sx={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block' }} />
                                    </Box>
                                    <Typography noWrap sx={{ mt: 0.75, fontSize: 13 }}>{asset.name}</Typography>
                                </ButtonBase>
                            ))}
                        </Box>
                    )
                )}

                {tab === 'snippets' && (
                    <Stack divider={<Box sx={{ borderTop: 1, borderColor: 'divider' }} />}>
                        {snippets.map((snippet) => (
                            <Stack key={snippet.id} direction="row" spacing={1} sx={{ py: 1.25, alignItems: 'flex-start' }}>
                                <Box sx={{ flex: 1, minWidth: 0 }}>
                                    <Typography sx={{ fontSize: 14, fontWeight: 600 }}>{snippet.name}</Typography>
                                    <Typography sx={{ fontSize: 13, color: 'text.secondary' }}>{snippet.text}</Typography>
                                </Box>
                                <Tooltip title={t('imageLibrary.editSnippet', { defaultValue: 'Edit' })}>
                                    <IconButton size="small" onClick={() => setSnippetDraft(snippet)} aria-label={t('imageLibrary.editSnippet', { defaultValue: 'Edit' })}>
                                        <Edit sx={{ fontSize: 18 }} />
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title={t('imageLibrary.deleteSnippet', { defaultValue: 'Delete' })}>
                                    <IconButton size="small" onClick={() => removeSnippet(snippet.id)} aria-label={t('imageLibrary.deleteSnippet', { defaultValue: 'Delete' })}>
                                        <DeleteOutline sx={{ fontSize: 18 }} />
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                        ))}
                        {snippets.length === 0 && (
                            <Typography sx={{ color: 'text.secondary', fontSize: 14, py: 6, textAlign: 'center' }}>
                                {t('imageLibrary.emptySnippets', { defaultValue: 'Snippets are pieces of prompt you reuse — a look, a lens, a palette. Insert one from the prompt field.' })}
                            </Typography>
                        )}
                    </Stack>
                )}
            </UnifiedCard>

            <Dialog open={openAsset !== null} onClose={() => setOpenAsset(null)} maxWidth="sm" fullWidth>
                {openAsset && (
                    <>
                        <DialogTitle sx={{ display: 'flex', alignItems: 'center', pr: 1, fontSize: '1.05rem' }}>
                            <Box component="span" sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>{openAsset.name}</Box>
                            <IconButton onClick={() => setOpenAsset(null)} aria-label={t('common.close', { defaultValue: 'Close' })}>
                                <Close />
                            </IconButton>
                        </DialogTitle>
                        <DialogContent dividers>
                            <Box sx={{ bgcolor: 'action.hover', borderRadius: 1.5, display: 'flex', justifyContent: 'center', mb: confirmingDelete ? 2 : 0 }}>
                                <Box component="img" src={openAsset.src} alt={openAsset.name} sx={{ maxWidth: '100%', maxHeight: 360, display: 'block' }} />
                            </Box>
                            {confirmingDelete && (
                                <Typography sx={{ fontSize: 13, color: 'error.main' }}>
                                    {t('imageLibrary.deleteHint', { defaultValue: 'Deletes the image and its file. Profiles that already use it keep their copy.' })}
                                </Typography>
                            )}
                        </DialogContent>
                        <DialogActions sx={{ px: 3, py: 1.5 }}>
                            {confirmingDelete ? (
                                <>
                                    <Button onClick={() => setConfirmingDelete(false)} sx={{ mr: 'auto' }}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                                    <Button color="error" variant="contained" onClick={() => deleteAsset(openAsset)}>
                                        {t('imageLibrary.confirmDelete', { defaultValue: 'Delete' })}
                                    </Button>
                                </>
                            ) : (
                                <>
                                    <Button color="error" startIcon={<DeleteOutline />} onClick={() => setConfirmingDelete(true)} sx={{ mr: 'auto' }}>
                                        {t('imageLibrary.delete', { defaultValue: 'Delete' })}
                                    </Button>
                                    <Button startIcon={<Download />} onClick={() => { void downloadImage(openAsset.src, openAsset.name.replace(/\.[a-z]+$/i, '')); }}>
                                        {t('playground.download', { defaultValue: 'Download' })}
                                    </Button>
                                </>
                            )}
                        </DialogActions>
                    </>
                )}
            </Dialog>

            <Dialog open={snippetDraft !== null} onClose={() => setSnippetDraft(null)} maxWidth="sm" fullWidth>
                <DialogTitle sx={{ fontSize: '1.05rem' }}>
                    {snippetDraft?.id
                        ? t('imageLibrary.editSnippetTitle', { defaultValue: 'Edit snippet' })
                        : t('imageLibrary.newSnippet', { defaultValue: 'New snippet' })}
                </DialogTitle>
                <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: '8px !important' }}>
                    <TextField
                        autoFocus
                        size="small"
                        label={t('imageLibrary.snippetName', { defaultValue: 'Name' })}
                        value={snippetDraft?.name ?? ''}
                        onChange={(event) => setSnippetDraft((draft) => ({ ...draft, name: event.target.value }))}
                    />
                    <TextField
                        multiline
                        minRows={3}
                        label={t('imageLibrary.snippetText', { defaultValue: 'Text' })}
                        value={snippetDraft?.text ?? ''}
                        onChange={(event) => setSnippetDraft((draft) => ({ ...draft, text: event.target.value }))}
                    />
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setSnippetDraft(null)}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                    <Button
                        variant="contained"
                        disabled={!snippetDraft?.text?.trim()}
                        onClick={() => {
                            if (!snippetDraft?.text?.trim()) return;
                            const text = snippetDraft.text.trim();
                            saveSnippet({ id: snippetDraft.id, name: snippetDraft.name?.trim() || text.slice(0, 8), text });
                            setSnippetDraft(null);
                        }}
                    >
                        {t('imageLibrary.saveSnippet', { defaultValue: 'Save' })}
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default ImageLibraryPage;
