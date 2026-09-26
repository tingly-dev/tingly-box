import { useMemo, useState } from 'react';
import {
    Box,
    Button,
    Chip,
    IconButton,
    InputAdornment,
    Paper,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import ConfirmDialog from '@/components/ConfirmDialog';
import { CopyIconButton } from '@/components/CopyIconButton';
import { Add, ContentCut, Delete, Edit, Search } from '@/components/icons';
import { useNotify } from '@/hooks/useNotify';
import {
    collectTags,
    deleteLibraryPrompt,
    matchesQuery,
    promptLabel,
    saveLibraryPrompt,
    saveLibraryPrompts,
    type LibraryPrompt,
    type LibraryPromptInput,
    type PromptPieceKind,
} from '@/utils/imageLibrary';
import LibraryPromptEditorDialog from './LibraryPromptEditorDialog';
import PromptSplitDialog from './PromptSplitDialog';
import { playgroundHandoffState } from './libraryHandoff';

type KindFilter = 'all' | PromptPieceKind;

interface LibraryPromptsPanelProps {
    prompts: LibraryPrompt[];
    loaded: boolean;
}

// Kept prompt material: whole prompts, and the terms and phrases taken out of
// them. Browsed by the two things people remember a piece by — what kind it
// is and what it is tagged — plus search for the words themselves.
const LibraryPromptsPanel: React.FC<LibraryPromptsPanelProps> = ({ prompts, loaded }) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { notify } = useNotify();
    const [query, setQuery] = useState('');
    const [kind, setKind] = useState<KindFilter>('all');
    const [tag, setTag] = useState<string | null>(null);
    // `undefined` closed, `null` a new piece, otherwise the one being edited.
    const [editing, setEditing] = useState<LibraryPrompt | null | undefined>(undefined);
    const [splitting, setSplitting] = useState<LibraryPrompt | null>(null);
    const [removing, setRemoving] = useState<LibraryPrompt | null>(null);
    // Bumped on every open, so each dialog mounts fresh from what it was
    // opened on rather than resetting itself in an effect.
    const [dialogSession, setDialogSession] = useState(0);
    const openEditor = (item: LibraryPrompt | null) => { setDialogSession((n) => n + 1); setEditing(item); };
    const openSplit = (item: LibraryPrompt) => { setDialogSession((n) => n + 1); setSplitting(item); };

    const knownTags = useMemo(() => collectTags(prompts), [prompts]);
    const byId = useMemo(() => new Map(prompts.map((item) => [item.id, item])), [prompts]);
    const visible = useMemo(() => prompts.filter((item) => (
        (kind === 'all' || item.kind === kind)
        && (!tag || item.tags.includes(tag))
        && matchesQuery([item.title, item.text, ...item.tags], query)
    )), [kind, prompts, query, tag]);
    const pieceCount = (id: string) => prompts.filter((item) => item.sourceId === id).length;

    const kindLabel = (value: PromptPieceKind) => (value === 'prompt'
        ? t('imageLibrary.kind.prompt', { defaultValue: 'Prompt' })
        : value === 'term'
            ? t('imageLibrary.kind.term', { defaultValue: 'Term' })
            : t('imageLibrary.kind.phrase', { defaultValue: 'Phrase' }));
    const failed = () => notify('error', t('imageLibrary.saveFailed', { defaultValue: 'Could not save to the library' }));

    const handleSave = async (input: LibraryPromptInput) => {
        if (await saveLibraryPrompt(input)) setEditing(undefined);
        else failed();
    };
    const handleSaveSplit = async (pieces: LibraryPromptInput[]) => {
        if (await saveLibraryPrompts(pieces)) {
            setSplitting(null);
            notify('success', t('imageLibrary.split.saved', { defaultValue: 'Saved {{count}} pieces', count: pieces.length }));
        } else {
            failed();
        }
    };
    // Using a piece goes to the playground with it: a whole prompt fills the
    // field, a term or phrase is added to it.
    const handleUse = (item: LibraryPrompt) => {
        navigate('/image/playground', {
            state: playgroundHandoffState(item.kind === 'prompt' ? { prompt: item.text } : { piece: item.text }),
        });
    };

    return (
        <Stack spacing={2}>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} sx={{ alignItems: { sm: 'center' } }}>
                <TextField
                    size="small"
                    placeholder={t('imageLibrary.searchPrompts', { defaultValue: 'Search text and tags' })}
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    sx={{ flex: 1, maxWidth: { sm: 360 } }}
                    slotProps={{
                        input: {
                            startAdornment: (
                                <InputAdornment position="start"><Search fontSize="small" /></InputAdornment>
                            ),
                        },
                    }}
                />
                <ToggleButtonGroup
                    size="small"
                    exclusive
                    value={kind}
                    onChange={(_, value: KindFilter | null) => { if (value) setKind(value); }}
                >
                    <ToggleButton value="all">{t('imageLibrary.kind.all', { defaultValue: 'All' })}</ToggleButton>
                    <ToggleButton value="prompt">{kindLabel('prompt')}</ToggleButton>
                    <ToggleButton value="term">{kindLabel('term')}</ToggleButton>
                    <ToggleButton value="phrase">{kindLabel('phrase')}</ToggleButton>
                </ToggleButtonGroup>
                <Box sx={{ flex: 1 }} />
                <Button variant="contained" startIcon={<Add />} onClick={() => openEditor(null)}>
                    {t('imageLibrary.new', { defaultValue: 'New' })}
                </Button>
            </Stack>

            {knownTags.length > 0 && (
                <Stack direction="row" spacing={0.75} useFlexGap sx={{ flexWrap: 'wrap' }}>
                    {knownTags.map((value) => (
                        <Chip
                            key={value}
                            size="small"
                            label={value}
                            variant={tag === value ? 'filled' : 'outlined'}
                            color={tag === value ? 'primary' : 'default'}
                            onClick={() => setTag((current) => (current === value ? null : value))}
                        />
                    ))}
                </Stack>
            )}

            {loaded && prompts.length === 0 && (
                <Paper variant="outlined" sx={{ p: 3, textAlign: 'center' }}>
                    <Typography variant="subtitle1" sx={{ mb: 1 }}>
                        {t('imageLibrary.emptyPromptsTitle', { defaultValue: 'Keep the prompts that worked — and the parts that made them work' })}
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 560, mx: 'auto' }}>
                        {t('imageLibrary.emptyPromptsBody', {
                            defaultValue: 'Save a prompt from the bookmark button on the Playground’s prompt field, or create one here. Then split it into terms ("rim lighting") and phrases ("a quiet street after rain") to reuse them in new prompts.',
                        })}
                    </Typography>
                </Paper>
            )}
            {loaded && prompts.length > 0 && visible.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ py: 3, textAlign: 'center' }}>
                    {t('imageLibrary.noMatches', { defaultValue: 'Nothing matches' })}
                </Typography>
            )}

            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 1.5 }}>
                {visible.map((item) => {
                    const source = item.sourceId ? byId.get(item.sourceId) : undefined;
                    const pieces = item.kind === 'prompt' ? pieceCount(item.id) : 0;
                    return (
                        <Paper key={item.id} variant="outlined" sx={{ p: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
                            <Stack direction="row" spacing={1} sx={{ alignItems: 'baseline', minWidth: 0 }}>
                                <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0 }}>
                                    {kindLabel(item.kind)}
                                </Typography>
                                {item.kind === 'prompt' && item.title.trim() && (
                                    <Typography variant="subtitle2" noWrap sx={{ minWidth: 0 }}>{item.title}</Typography>
                                )}
                            </Stack>
                            <Typography
                                variant={item.kind === 'term' ? 'subtitle1' : 'body2'}
                                sx={{
                                    whiteSpace: 'pre-wrap',
                                    wordBreak: 'break-word',
                                    display: '-webkit-box',
                                    WebkitLineClamp: 5,
                                    WebkitBoxOrient: 'vertical',
                                    overflow: 'hidden',
                                }}
                            >
                                {item.text}
                            </Typography>
                            {(item.tags.length > 0 || source || pieces > 0) && (
                                <Stack direction="row" spacing={0.5} useFlexGap sx={{ flexWrap: 'wrap', alignItems: 'center' }}>
                                    {item.tags.map((value) => (
                                        <Chip key={value} size="small" label={value} onClick={() => setTag(value)} />
                                    ))}
                                    {source && (
                                        <Typography variant="caption" color="text.disabled" noWrap sx={{ maxWidth: '100%' }}>
                                            {t('imageLibrary.fromPrompt', { defaultValue: 'from "{{name}}"', name: promptLabel(source) })}
                                        </Typography>
                                    )}
                                    {pieces > 0 && (
                                        <Typography variant="caption" color="text.disabled">
                                            {t('imageLibrary.pieceCount', { defaultValue: '{{count}} pieces kept', count: pieces })}
                                        </Typography>
                                    )}
                                </Stack>
                            )}
                            <Stack direction="row" spacing={0.25} sx={{ alignItems: 'center', mt: 'auto' }}>
                                <Button size="small" onClick={() => handleUse(item)}>
                                    {item.kind === 'prompt'
                                        ? t('imageLibrary.useInPlayground', { defaultValue: 'Use in Playground' })
                                        : t('imageLibrary.addToPrompt', { defaultValue: 'Add to prompt' })}
                                </Button>
                                <Box sx={{ flex: 1 }} />
                                <CopyIconButton
                                    value={item.text}
                                    label={t('common.copy', { defaultValue: 'Copy' })}
                                    copiedLabel={t('common.copied', { defaultValue: 'Copied!' })}
                                    iconSize={16}
                                />
                                {item.kind === 'prompt' && (
                                    <Tooltip title={t('imageLibrary.split.action', { defaultValue: 'Split into terms and phrases' })}>
                                        <IconButton
                                            size="small"
                                            onClick={() => openSplit(item)}
                                            aria-label={t('imageLibrary.split.action', { defaultValue: 'Split into terms and phrases' })}
                                        >
                                            <ContentCut sx={{ fontSize: 16 }} />
                                        </IconButton>
                                    </Tooltip>
                                )}
                                <Tooltip title={t('imageLibrary.editTitle', { defaultValue: 'Edit' })}>
                                    <IconButton
                                        size="small"
                                        onClick={() => openEditor(item)}
                                        aria-label={t('imageLibrary.editTitle', { defaultValue: 'Edit' })}
                                    >
                                        <Edit sx={{ fontSize: 16 }} />
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title={t('common.delete', { defaultValue: 'Delete' })}>
                                    <IconButton
                                        size="small"
                                        onClick={() => setRemoving(item)}
                                        aria-label={t('common.delete', { defaultValue: 'Delete' })}
                                    >
                                        <Delete sx={{ fontSize: 16 }} />
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                        </Paper>
                    );
                })}
            </Box>

            <LibraryPromptEditorDialog
                key={`editor-${dialogSession}`}
                open={editing !== undefined}
                initial={editing ?? null}
                knownTags={knownTags}
                onClose={() => setEditing(undefined)}
                onSave={(input) => { void handleSave(input); }}
            />
            <PromptSplitDialog
                key={`split-${dialogSession}`}
                source={splitting}
                existing={prompts}
                knownTags={knownTags}
                onClose={() => setSplitting(null)}
                onSave={(pieces) => { void handleSaveSplit(pieces); }}
            />
            <ConfirmDialog
                open={removing !== null}
                title={t('imageLibrary.deletePromptTitle', { defaultValue: 'Delete "{{name}}"?', name: removing ? promptLabel(removing) : '' })}
                description={removing && pieceCount(removing.id) > 0
                    ? t('imageLibrary.deletePromptKeepsPieces', {
                        defaultValue: 'Removes it from the library. The pieces split from it are kept.',
                    })
                    : t('imageLibrary.deletePromptBody', { defaultValue: 'Removes it from the library.' })}
                confirmLabel={t('common.delete', { defaultValue: 'Delete' })}
                cancelLabel={t('common.cancel', { defaultValue: 'Cancel' })}
                confirmColor="error"
                onClose={() => setRemoving(null)}
                onConfirm={() => {
                    const target = removing;
                    setRemoving(null);
                    if (target) void deleteLibraryPrompt(target.id);
                }}
            />
        </Stack>
    );
};

export default LibraryPromptsPanel;
