import { useMemo, useState } from 'react';
import { Box, Button, Chip, IconButton, Paper, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import ConfirmDialog from '@/components/ConfirmDialog';
import { CopyIconButton } from '@/components/CopyIconButton';
import { Add, ContentCut, Delete, Edit } from '@/components/icons';
import { useNotify } from '@/hooks/useNotify';
import { KindToggle, usePieceKindLabel } from './fields';
import { handoffState } from './handoff';
import { EmptyState, NoMatches, SearchField } from './LibraryChrome';
import { PIECE_KINDS, collectTags, matchesQuery, pieceLabel, type PieceInput, type PieceKind, type PromptPiece } from './model';
import PieceEditorDialog from './PieceEditorDialog';
import SplitDialog from './SplitDialog';
import { deletePiece, savePieces } from './store';

const FILTER_KINDS: Array<PieceKind | 'all'> = ['all', ...PIECE_KINDS];

interface PieceCardProps {
    piece: PromptPiece;
    source?: PromptPiece;
    splitCount: number;
    onTag: (tag: string) => void;
    onUse: () => void;
    onSplit: () => void;
    onEdit: () => void;
    onDelete: () => void;
}

const PieceCard: React.FC<PieceCardProps> = ({ piece, source, splitCount, onTag, onUse, onSplit, onEdit, onDelete }) => {
    const { t } = useTranslation();
    const kindLabel = usePieceKindLabel();
    const actions = [
        piece.kind === 'prompt' && {
            label: t('imageLibrary.split.action', { defaultValue: 'Split into terms and phrases' }),
            icon: <ContentCut sx={{ fontSize: 16 }} />,
            onClick: onSplit,
        },
        { label: t('imageLibrary.editTitle', { defaultValue: 'Edit' }), icon: <Edit sx={{ fontSize: 16 }} />, onClick: onEdit },
        { label: t('common.delete', { defaultValue: 'Delete' }), icon: <Delete sx={{ fontSize: 16 }} />, onClick: onDelete },
    ].filter((action) => action !== false);
    return (
        <Paper variant="outlined" sx={{ p: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Stack direction="row" spacing={1} sx={{ alignItems: 'baseline', minWidth: 0 }}>
                <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0 }}>{kindLabel(piece.kind)}</Typography>
                {piece.title && <Typography variant="subtitle2" noWrap sx={{ minWidth: 0 }}>{piece.title}</Typography>}
            </Stack>
            <Typography
                variant={piece.kind === 'term' ? 'subtitle1' : 'body2'}
                sx={{
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                    display: '-webkit-box',
                    WebkitLineClamp: 5,
                    WebkitBoxOrient: 'vertical',
                    overflow: 'hidden',
                }}
            >
                {piece.text}
            </Typography>
            {(piece.tags.length > 0 || source || splitCount > 0) && (
                <Stack direction="row" spacing={0.5} useFlexGap sx={{ flexWrap: 'wrap', alignItems: 'center' }}>
                    {piece.tags.map((tag) => <Chip key={tag} size="small" label={tag} onClick={() => onTag(tag)} />)}
                    <Typography variant="caption" color="text.disabled" noWrap sx={{ maxWidth: '100%' }}>
                        {source
                            ? t('imageLibrary.fromPrompt', { defaultValue: 'from "{{name}}"', name: pieceLabel(source) })
                            : splitCount > 0 && t('imageLibrary.pieceCount', { defaultValue: '{{count}} pieces kept', count: splitCount })}
                    </Typography>
                </Stack>
            )}
            <Stack direction="row" spacing={0.25} sx={{ alignItems: 'center', mt: 'auto' }}>
                <Button size="small" onClick={onUse}>
                    {piece.kind === 'prompt'
                        ? t('imageLibrary.useInPlayground', { defaultValue: 'Use in Playground' })
                        : t('imageLibrary.addToPrompt', { defaultValue: 'Add to prompt' })}
                </Button>
                <Box sx={{ flex: 1 }} />
                <CopyIconButton
                    value={piece.text}
                    label={t('common.copy', { defaultValue: 'Copy' })}
                    copiedLabel={t('common.copied', { defaultValue: 'Copied!' })}
                    iconSize={16}
                />
                {actions.map((action) => (
                    <Tooltip key={action.label} title={action.label}>
                        <IconButton size="small" onClick={action.onClick} aria-label={action.label}>{action.icon}</IconButton>
                    </Tooltip>
                ))}
            </Stack>
        </Paper>
    );
};

// Kept prompt material: whole prompts, and the terms and phrases taken out of
// them. Browsed by what people remember a piece by — its kind and its tags —
// plus search for the words themselves.
const PiecesPanel: React.FC<{ pieces: PromptPiece[]; loaded: boolean }> = ({ pieces, loaded }) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { notify } = useNotify();
    const [query, setQuery] = useState('');
    const [kind, setKind] = useState<PieceKind | 'all'>('all');
    const [tag, setTag] = useState<string | null>(null);
    // Which dialog is showing and on what. Closing only clears `open`, so a
    // dialog keeps its content while it fades out; `session` gives each
    // opening a fresh `key`, so it starts from what it was opened on.
    const [dialog, setDialog] = useState<{
        type: 'edit' | 'split' | 'delete';
        piece: PromptPiece | null;
        open: boolean;
        session: number;
    }>({ type: 'edit', piece: null, open: false, session: 0 });
    const open = (type: typeof dialog.type, piece: PromptPiece | null) => {
        setDialog((current) => ({ type, piece, open: true, session: current.session + 1 }));
    };
    const close = () => setDialog((current) => ({ ...current, open: false }));
    const showing = (type: typeof dialog.type) => dialog.open && dialog.type === type;

    const knownTags = useMemo(() => collectTags(pieces), [pieces]);
    const visible = pieces.filter((piece) => (kind === 'all' || piece.kind === kind)
        && (!tag || piece.tags.includes(tag))
        && matchesQuery([piece.title, piece.text, ...piece.tags], query));

    const save = async (inputs: PieceInput[], successMessage?: string) => {
        if (!(await savePieces(inputs))) {
            notify('error', t('imageLibrary.saveFailed', { defaultValue: 'Could not save to the library' }));
            return;
        }
        close();
        if (successMessage) notify('success', successMessage);
    };
    // Using a piece goes to the playground with it: a whole prompt fills the
    // field, a term or phrase is added to it.
    const use = (piece: PromptPiece) => navigate('/image/playground', {
        state: handoffState(piece.kind === 'prompt' ? { prompt: piece.text } : { piece: piece.text }),
    });
    const splitCount = (id: string) => pieces.filter((piece) => piece.sourceId === id).length;

    return (
        <Stack spacing={2}>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} sx={{ alignItems: { sm: 'center' } }}>
                <SearchField
                    value={query}
                    onChange={setQuery}
                    placeholder={t('imageLibrary.searchPrompts', { defaultValue: 'Search text and tags' })}
                />
                <KindToggle kinds={FILTER_KINDS} value={kind} onChange={setKind} />
                <Box sx={{ flex: 1 }} />
                <Button variant="contained" startIcon={<Add />} onClick={() => open('edit', null)}>
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

            {loaded && pieces.length === 0 && (
                <EmptyState
                    title={t('imageLibrary.emptyPromptsTitle', { defaultValue: 'Keep the prompts that worked — and the parts that made them work' })}
                    body={t('imageLibrary.emptyPromptsBody', {
                        defaultValue: 'Save a prompt from the bookmark button on the Playground’s prompt field, or create one here. Then split it into terms ("rim lighting") and phrases ("a quiet street after rain") to reuse them in new prompts.',
                    })}
                />
            )}
            {loaded && pieces.length > 0 && visible.length === 0 && <NoMatches />}

            <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))', gap: 1.5 }}>
                {visible.map((piece) => (
                    <PieceCard
                        key={piece.id}
                        piece={piece}
                        source={piece.sourceId ? pieces.find((item) => item.id === piece.sourceId) : undefined}
                        splitCount={piece.kind === 'prompt' ? splitCount(piece.id) : 0}
                        onTag={setTag}
                        onUse={() => use(piece)}
                        onSplit={() => open('split', piece)}
                        onEdit={() => open('edit', piece)}
                        onDelete={() => open('delete', piece)}
                    />
                ))}
            </Box>

            <PieceEditorDialog
                key={`edit-${dialog.session}`}
                open={showing('edit')}
                initial={dialog.type === 'edit' ? dialog.piece : null}
                knownTags={knownTags}
                onClose={close}
                onSave={(input) => { void save([input]); }}
            />
            <SplitDialog
                key={`split-${dialog.session}`}
                open={showing('split')}
                source={dialog.type === 'split' ? dialog.piece : null}
                existing={pieces}
                knownTags={knownTags}
                onClose={close}
                onSave={(inputs) => {
                    void save(inputs, t('imageLibrary.split.saved', { defaultValue: 'Saved {{count}} pieces', count: inputs.length }));
                }}
            />
            <ConfirmDialog
                open={showing('delete')}
                title={t('imageLibrary.deletePromptTitle', {
                    defaultValue: 'Delete "{{name}}"?',
                    name: dialog.type === 'delete' && dialog.piece ? pieceLabel(dialog.piece) : '',
                })}
                description={dialog.type === 'delete' && dialog.piece && splitCount(dialog.piece.id) > 0
                    ? t('imageLibrary.deletePromptKeepsPieces', { defaultValue: 'Removes it from the library. The pieces split from it are kept.' })
                    : t('imageLibrary.deletePromptBody', { defaultValue: 'Removes it from the library.' })}
                confirmLabel={t('common.delete', { defaultValue: 'Delete' })}
                cancelLabel={t('common.cancel', { defaultValue: 'Cancel' })}
                confirmColor="error"
                onClose={close}
                onConfirm={() => {
                    if (dialog.type === 'delete' && dialog.piece) void deletePiece(dialog.piece.id);
                    close();
                }}
            />
        </Stack>
    );
};

export default PiecesPanel;
