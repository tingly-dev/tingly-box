import { useMemo, useState } from 'react';
import {
    Autocomplete,
    Box,
    Button,
    Checkbox,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    InputBase,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Add, Close } from '@/components/icons';
import {
    normalizeTags,
    suggestPromptPieces,
    type LibraryPrompt,
    type LibraryPromptInput,
    type PromptPieceCandidate,
} from '@/utils/imageLibrary';

interface Row extends PromptPieceCandidate {
    key: number;
    keep: boolean;
}

interface PromptSplitDialogProps {
    // The whole prompt being taken apart; null when closed.
    source: LibraryPrompt | null;
    // Pieces already kept, so a candidate that is already one starts unticked.
    existing: LibraryPrompt[];
    knownTags: string[];
    onClose: () => void;
    onSave: (pieces: LibraryPromptInput[]) => void;
}

// Splitting a prompt into the terms and phrases worth keeping. The first cut
// is mechanical (punctuation, see suggestPromptPieces) and everything after
// it is the person's call: which pieces matter, whether each is a term or a
// phrase, how it reads on its own, what to tag it. The candidate list is the
// interface an AI splitter will fill later; the review stays either way.
//
// The candidates are cut once, when the dialog mounts; the caller gives it a
// new `key` each time it opens.
const PromptSplitDialog: React.FC<PromptSplitDialogProps> = ({ source, existing, knownTags, onClose, onSave }) => {
    const { t } = useTranslation();
    const [rows, setRows] = useState<Row[]>(() => {
        if (!source) return [];
        const kept = new Set(existing.filter((item) => item.kind !== 'prompt').map((item) => item.text.trim().toLowerCase()));
        return suggestPromptPieces(source.text).map((candidate, key) => ({
            ...candidate,
            key,
            keep: !kept.has(candidate.text.toLowerCase()),
        }));
    });
    const [tags, setTags] = useState<string[]>(source?.tags ?? []);

    const update = (key: number, patch: Partial<Row>) => {
        setRows((current) => current.map((row) => (row.key === key ? { ...row, ...patch } : row)));
    };
    const kept = useMemo(() => rows.filter((row) => row.keep && row.text.trim()), [rows]);

    const save = () => {
        if (!source) return;
        onSave(kept.map((row) => ({
            kind: row.kind,
            text: row.text,
            tags,
            sourceId: source.id,
        })));
    };

    return (
        <Dialog open={source !== null} onClose={onClose} maxWidth="md" fullWidth>
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
                            <Stack
                                key={row.key}
                                direction="row"
                                spacing={1}
                                sx={{ alignItems: 'center', opacity: row.keep ? 1 : 0.5 }}
                            >
                                <Checkbox
                                    size="small"
                                    checked={row.keep}
                                    onChange={(event) => update(row.key, { keep: event.target.checked })}
                                    slotProps={{ input: { 'aria-label': row.text } }}
                                />
                                <InputBase
                                    value={row.text}
                                    onChange={(event) => update(row.key, { text: event.target.value })}
                                    multiline
                                    sx={{
                                        flex: 1,
                                        px: 1,
                                        py: 0.5,
                                        fontSize: 14,
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 1,
                                    }}
                                />
                                <ToggleButtonGroup
                                    size="small"
                                    exclusive
                                    value={row.kind}
                                    onChange={(_, value: Row['kind'] | null) => { if (value) update(row.key, { kind: value }); }}
                                    sx={{ flexShrink: 0 }}
                                >
                                    <ToggleButton value="term" sx={{ py: 0.25, px: 1 }}>
                                        {t('imageLibrary.kind.term', { defaultValue: 'Term' })}
                                    </ToggleButton>
                                    <ToggleButton value="phrase" sx={{ py: 0.25, px: 1 }}>
                                        {t('imageLibrary.kind.phrase', { defaultValue: 'Phrase' })}
                                    </ToggleButton>
                                </ToggleButtonGroup>
                                <Tooltip title={t('common.delete', { defaultValue: 'Delete' })}>
                                    <IconButton
                                        size="small"
                                        onClick={() => setRows((current) => current.filter((item) => item.key !== row.key))}
                                        aria-label={t('common.delete', { defaultValue: 'Delete' })}
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
                    <Autocomplete
                        multiple
                        freeSolo
                        size="small"
                        options={knownTags}
                        value={tags}
                        onChange={(_, value) => setTags(normalizeTags(value))}
                        renderValue={(value, getItemProps) => value.map((option, index) => {
                            const { key, ...itemProps } = getItemProps({ index });
                            return <Chip key={key} size="small" label={option} {...itemProps} />;
                        })}
                        renderInput={(params) => (
                            <TextField
                                {...params}
                                label={t('imageLibrary.split.tagsLabel', { defaultValue: 'Tags for every saved piece' })}
                            />
                        )}
                    />
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                <Button onClick={onClose}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                <Button variant="contained" disabled={kept.length === 0} onClick={save}>
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

export default PromptSplitDialog;
