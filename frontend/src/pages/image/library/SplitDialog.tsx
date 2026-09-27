import { useState } from 'react';
import {
    Box,
    Button,
    Checkbox,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    InputBase,
    Stack,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Add, Close } from '@/components/icons';
import { KindToggle, TagInput } from './fields';
import { suggestPieces, type PieceCandidate, type PieceInput, type PromptPiece } from './model';

interface Row extends PieceCandidate {
    key: number;
    keep: boolean;
}

const CANDIDATE_KINDS: Array<PieceCandidate['kind']> = ['term', 'phrase'];

interface SplitDialogProps {
    open: boolean;
    // The whole prompt being taken apart; null when closed. Read once on
    // mount: the caller gives the dialog a new `key` each time it opens.
    source: PromptPiece | null;
    // Pieces already kept, so a candidate that is already one starts unticked.
    existing: PromptPiece[];
    knownTags: string[];
    onClose: () => void;
    onSave: (pieces: PieceInput[]) => void;
}

// Splitting a prompt into the terms and phrases worth keeping. The first cut
// is mechanical (suggestPieces) and everything after it is the person's call:
// which pieces matter, whether each is a term or a phrase, how it reads on its
// own, what to tag it. An AI splitter would fill the same rows; the review
// stays either way.
const SplitDialog: React.FC<SplitDialogProps> = ({ open, source, existing, knownTags, onClose, onSave }) => {
    const { t } = useTranslation();
    const [rows, setRows] = useState<Row[]>(() => {
        const kept = new Set(existing.filter((piece) => piece.kind !== 'prompt').map((piece) => piece.text.toLowerCase()));
        return suggestPieces(source?.text ?? '').map((candidate, key) => ({
            ...candidate,
            key,
            keep: !kept.has(candidate.text.toLowerCase()),
        }));
    });
    const [tags, setTags] = useState<string[]>(source?.tags ?? []);

    const update = (key: number, patch: Partial<Row>) => {
        setRows((current) => current.map((row) => (row.key === key ? { ...row, ...patch } : row)));
    };
    const kept = rows.filter((row) => row.keep && row.text.trim());
    const deleteLabel = t('common.delete', { defaultValue: 'Delete' });

    return (
        <Dialog open={open && source !== null} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle>{t('imageLibrary.split.title', { defaultValue: 'Split into terms and phrases' })}</DialogTitle>
            <DialogContent dividers>
                <Stack spacing={2}>
                    <Box sx={{ p: 1.5, borderRadius: 1, bgcolor: 'action.hover', maxHeight: 140, overflowY: 'auto' }}>
                        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>{source?.text}</Typography>
                    </Box>
                    <Typography variant="caption" color="text.secondary">
                        {t('imageLibrary.split.hint', {
                            defaultValue: 'A first cut at punctuation. Untick what is not worth keeping, fix the wording so each piece stands on its own, and mark it as a term or a phrase.',
                        })}
                    </Typography>
                    <Stack spacing={0.75}>
                        {rows.map((row) => (
                            <Stack key={row.key} direction="row" spacing={1} sx={{ alignItems: 'center', opacity: row.keep ? 1 : 0.5 }}>
                                <Checkbox
                                    size="small"
                                    checked={row.keep}
                                    onChange={(event) => update(row.key, { keep: event.target.checked })}
                                    slotProps={{ input: { 'aria-label': row.text } }}
                                />
                                <InputBase
                                    multiline
                                    value={row.text}
                                    onChange={(event) => update(row.key, { text: event.target.value })}
                                    sx={{ flex: 1, px: 1, py: 0.5, fontSize: 14, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}
                                />
                                <KindToggle
                                    kinds={CANDIDATE_KINDS}
                                    value={row.kind}
                                    onChange={(kind) => update(row.key, { kind })}
                                    sx={{ flexShrink: 0, '& .MuiToggleButton-root': { py: 0.25, px: 1 } }}
                                />
                                <Tooltip title={deleteLabel}>
                                    <IconButton
                                        size="small"
                                        onClick={() => setRows((current) => current.filter((item) => item.key !== row.key))}
                                        aria-label={deleteLabel}
                                    >
                                        <Close sx={{ fontSize: 16 }} />
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                        ))}
                        <Box>
                            <Button
                                size="small"
                                startIcon={<Add />}
                                onClick={() => setRows((current) => [
                                    ...current,
                                    { key: Math.max(-1, ...current.map((row) => row.key)) + 1, text: '', kind: 'term', keep: true },
                                ])}
                            >
                                {t('imageLibrary.split.addRow', { defaultValue: 'Add a piece' })}
                            </Button>
                        </Box>
                    </Stack>
                    <TagInput
                        value={tags}
                        onChange={setTags}
                        options={knownTags}
                        label={t('imageLibrary.split.tagsLabel', { defaultValue: 'Tags for every saved piece' })}
                    />
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                <Button onClick={onClose}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                <Button
                    variant="contained"
                    disabled={!source || kept.length === 0}
                    onClick={() => source && onSave(kept.map((row) => ({ kind: row.kind, text: row.text, tags, sourceId: source.id })))}
                >
                    {t('imageLibrary.split.save', {
                        defaultValue_one: 'Save {{count}} piece',
                        defaultValue_other: 'Save {{count}} pieces',
                        count: kept.length,
                    })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default SplitDialog;
