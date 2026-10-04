import { useEffect, useMemo, useState } from 'react';
import {
    Box,
    Button,
    ButtonBase,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Link,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate, useSearchParams } from 'react-router-dom';
import UnifiedCard from '@/components/UnifiedCard';
import { CopyIconButton } from '@/components/CopyIconButton';
import { Add, Close, DeleteOutline, Download, Edit } from '@/components/icons';
import { api } from '@/services/api';
import { fontMono } from '@/theme/fonts';
import { downloadImage } from '@/utils/download';
import type { ImageAsset, PromptSnippet } from './library/assetTypes';
import { removeAsset, removeSnippet, saveSnippet, useAssets, useSnippets } from './library/assetStore';
import { profilesUsing } from './library/assetUsage';
import { updateImageProfile, useImageProfiles } from './profiles/imageProfileStore';

type Tab = 'images' | 'snippets';

// The library (素材库): what the user chose to keep — images (kept results and
// the references their profiles use) and prompt snippets. History is what
// happened and can be cleared; this is what stays. See .design/image-library.md.
const ImageLibraryPage: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [params, setParams] = useSearchParams();
    const tab: Tab = params.get('tab') === 'snippets' ? 'snippets' : 'images';
    const assets = useAssets();
    const snippets = useSnippets();
    const profiles = useImageProfiles();
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

    const usage = useMemo(() => new Map(assets.map((asset) => [asset.id, profilesUsing(asset, profiles)])), [assets, profiles]);
    const openUsage = openAsset ? usage.get(openAsset.id) ?? [] : [];

    const usageLine = (asset: ImageAsset) => {
        const users = usage.get(asset.id) ?? [];
        if (users.length === 0) return t('imageLibrary.unused', { defaultValue: 'Not used by any profile' });
        if (users.length === 1) return t('imageLibrary.usedByOne', { defaultValue: 'Used in {{name}}', name: users[0].name });
        return t('imageLibrary.usedByMany', { defaultValue: 'Used in {{name}} and {{count}} more', name: users[0].name, count: users.length - 1 });
    };
    const originLabel = (asset: ImageAsset) => (asset.origin === 'generated'
        ? t('imageLibrary.originGenerated', { defaultValue: 'Generated' })
        : t('imageLibrary.originReference', { defaultValue: 'Reference' }));

    const deleteAsset = (asset: ImageAsset) => {
        // A deleted image leaves the profiles that used it too — a profile
        // pointing at an image that no longer exists helps no one.
        for (const profile of usage.get(asset.id) ?? []) {
            updateImageProfile(profile.id, { refs: profile.refs.filter((ref) => ref.previewUrl !== asset.src) });
        }
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
                            {t('imageLibrary.emptyImages', { defaultValue: 'Nothing kept yet. Open a result in the Playground and choose “Keep”; references you add to a profile land here too.' })}
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
                                    <Typography
                                        noWrap
                                        sx={{ fontSize: 12, color: (usage.get(asset.id)?.length ?? 0) > 0 ? 'text.secondary' : 'text.disabled' }}
                                    >
                                        {originLabel(asset)} · {usageLine(asset)}
                                    </Typography>
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
                            <Box sx={{ bgcolor: 'action.hover', borderRadius: 1.5, display: 'flex', justifyContent: 'center', mb: 2 }}>
                                <Box component="img" src={openAsset.src} alt={openAsset.name} sx={{ maxWidth: '100%', maxHeight: 360, display: 'block' }} />
                            </Box>
                            <Typography sx={{ fontSize: 13, color: 'text.secondary' }}>
                                {originLabel(openAsset)}
                                {openAsset.width && openAsset.height ? ` · ${openAsset.width}×${openAsset.height}` : ''}
                                {` · ${new Date(openAsset.createdAt).toLocaleDateString()}`}
                            </Typography>
                            <Typography sx={{ fontSize: 13, mt: 1 }}>
                                {openUsage.length === 0
                                    ? t('imageLibrary.unused', { defaultValue: 'Not used by any profile' })
                                    : (
                                        <>
                                            {t('imageLibrary.usedIn', { defaultValue: 'Used in' })}{' '}
                                            {openUsage.map((profile, index) => (
                                                <span key={profile.id}>
                                                    {index > 0 && '、'}
                                                    <Link component="button" type="button" onClick={() => navigate(`/image/profile/${profile.id}`)} sx={{ fontSize: 13, verticalAlign: 'baseline' }}>
                                                        {profile.name}
                                                    </Link>
                                                </span>
                                            ))}
                                        </>
                                    )}
                            </Typography>
                            {/* What deleting takes with it is said before it
                                happens, in place — not after, in a toast. */}
                            {confirmingDelete && (
                                <Typography sx={{ mt: 2, fontSize: 13, color: 'error.main' }}>
                                    {openUsage.length > 0
                                        ? t('imageLibrary.deleteUsed', { defaultValue: 'Deleting also removes it from {{count}} profile(s) that use it, and deletes the file.', count: openUsage.length })
                                        : t('imageLibrary.deleteUnused', { defaultValue: 'Deletes the file. This cannot be undone.' })}
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
