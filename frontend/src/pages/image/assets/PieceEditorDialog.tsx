import { useState } from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { KindToggle, TagInput } from './fields';
import { PIECE_KINDS, type PieceInput, type PieceKind, type PromptPiece } from './model';

interface PieceEditorDialogProps {
    open: boolean;
    // The piece being edited, or null for a new one. Read once on mount: the
    // caller gives the dialog a new `key` each time it opens.
    initial: PromptPiece | null;
    knownTags: string[];
    onClose: () => void;
    onSave: (input: PieceInput) => void;
}

// One form for all three kinds. The kind is a field rather than three "new"
// buttons: it is one axis of the same thing, and it can change later (a
// phrase that turned out to be a term).
const PieceEditorDialog: React.FC<PieceEditorDialogProps> = ({ open, initial, knownTags, onClose, onSave }) => {
    const { t } = useTranslation();
    const [kind, setKind] = useState<PieceKind>(initial?.kind ?? 'prompt');
    const [title, setTitle] = useState(initial?.title ?? '');
    const [text, setText] = useState(initial?.text ?? '');
    const [tags, setTags] = useState<string[]>(initial?.tags ?? []);

    const save = () => {
        if (text.trim()) onSave({ id: initial?.id, sourceId: initial?.sourceId, kind, title, text, tags });
    };
    const hint = {
        prompt: t('imageAssets.kindHint.prompt', { defaultValue: 'A whole prompt — using it replaces the prompt field.' }),
        term: t('imageAssets.kindHint.term', { defaultValue: 'A keyword such as "rim lighting" or "35mm" — using it adds it to the prompt.' }),
        phrase: t('imageAssets.kindHint.phrase', { defaultValue: 'A descriptive sentence or clause — using it adds it to the prompt.' }),
    }[kind];

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                {initial
                    ? t('imageAssets.editTitle', { defaultValue: 'Edit' })
                    : t('imageAssets.newTitle', { defaultValue: 'New prompt material' })}
            </DialogTitle>
            <DialogContent dividers>
                <Stack spacing={2}>
                    <Stack spacing={0.5}>
                        <KindToggle kinds={PIECE_KINDS} value={kind} onChange={setKind} />
                        <Typography variant="caption" color="text.secondary">{hint}</Typography>
                    </Stack>
                    {kind === 'prompt' && (
                        <TextField
                            size="small"
                            label={t('imageAssets.titleLabel', { defaultValue: 'Title (optional)' })}
                            value={title}
                            onChange={(event) => setTitle(event.target.value)}
                        />
                    )}
                    <TextField
                        autoFocus
                        multiline
                        minRows={kind === 'term' ? 1 : 4}
                        maxRows={16}
                        label={t('imageAssets.textLabel', { defaultValue: 'Text' })}
                        value={text}
                        onChange={(event) => setText(event.target.value)}
                        onKeyDown={(event) => {
                            if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
                                event.preventDefault();
                                save();
                            }
                        }}
                    />
                    <TagInput
                        value={tags}
                        onChange={setTags}
                        options={knownTags}
                        label={t('imageAssets.tagsLabel', { defaultValue: 'Tags' })}
                    />
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, py: 1.5 }}>
                <Button onClick={onClose}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                <Button variant="contained" disabled={!text.trim()} onClick={save}>
                    {t('common.save', { defaultValue: 'Save' })}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default PieceEditorDialog;
