import { useState } from 'react';
import {
    Autocomplete,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { normalizeTags, type LibraryPrompt, type LibraryPromptInput, type PromptPieceKind } from '@/utils/imageLibrary';

interface LibraryPromptEditorDialogProps {
    open: boolean;
    // The piece being edited, or null for a new one.
    initial: LibraryPrompt | null;
    // Tags already in use, offered as suggestions so the vocabulary stays small.
    knownTags: string[];
    onClose: () => void;
    onSave: (input: LibraryPromptInput) => void;
}

// The form reads `initial` once when it mounts; the caller gives it a new
// `key` each time it opens.
//
// One form for all three kinds. The kind is picked here rather than by three
// different "new" buttons: it is one axis of the same thing, and it can be
// changed later (a phrase that turned out to be a term).
const LibraryPromptEditorDialog: React.FC<LibraryPromptEditorDialogProps> = ({
    open,
    initial,
    knownTags,
    onClose,
    onSave,
}) => {
    const { t } = useTranslation();
    const [kind, setKind] = useState<PromptPieceKind>(initial?.kind ?? 'prompt');
    const [title, setTitle] = useState(initial?.title ?? '');
    const [text, setText] = useState(initial?.text ?? '');
    const [tags, setTags] = useState<string[]>(initial?.tags ?? []);

    const save = () => {
        if (!text.trim()) return;
        onSave({
            ...(initial ? { id: initial.id, createdAt: initial.createdAt, sourceId: initial.sourceId } : {}),
            kind,
            // Terms and phrases are their own name.
            title: kind === 'prompt' ? title : '',
            text,
            tags,
        });
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                {initial
                    ? t('imageLibrary.editTitle', { defaultValue: 'Edit' })
                    : t('imageLibrary.newTitle', { defaultValue: 'New prompt material' })}
            </DialogTitle>
            <DialogContent dividers>
                <Stack spacing={2}>
                    <Stack spacing={0.5}>
                        <ToggleButtonGroup
                            size="small"
                            exclusive
                            value={kind}
                            onChange={(_, value: PromptPieceKind | null) => { if (value) setKind(value); }}
                        >
                            <ToggleButton value="prompt">{t('imageLibrary.kind.prompt', { defaultValue: 'Prompt' })}</ToggleButton>
                            <ToggleButton value="term">{t('imageLibrary.kind.term', { defaultValue: 'Term' })}</ToggleButton>
                            <ToggleButton value="phrase">{t('imageLibrary.kind.phrase', { defaultValue: 'Phrase' })}</ToggleButton>
                        </ToggleButtonGroup>
                        <Typography variant="caption" color="text.secondary">
                            {kind === 'prompt'
                                ? t('imageLibrary.kindHint.prompt', { defaultValue: 'A whole prompt — using it replaces the prompt field.' })
                                : kind === 'term'
                                    ? t('imageLibrary.kindHint.term', { defaultValue: 'A keyword such as "rim lighting" or "35mm" — using it adds it to the prompt.' })
                                    : t('imageLibrary.kindHint.phrase', { defaultValue: 'A descriptive sentence or clause — using it adds it to the prompt.' })}
                        </Typography>
                    </Stack>
                    {kind === 'prompt' && (
                        <TextField
                            size="small"
                            label={t('imageLibrary.titleLabel', { defaultValue: 'Title (optional)' })}
                            value={title}
                            onChange={(event) => setTitle(event.target.value)}
                        />
                    )}
                    <TextField
                        autoFocus
                        multiline
                        minRows={kind === 'term' ? 1 : 4}
                        maxRows={16}
                        label={t('imageLibrary.textLabel', { defaultValue: 'Text' })}
                        value={text}
                        onChange={(event) => setText(event.target.value)}
                        onKeyDown={(event) => {
                            if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
                                event.preventDefault();
                                save();
                            }
                        }}
                    />
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
                                label={t('imageLibrary.tagsLabel', { defaultValue: 'Tags' })}
                                placeholder={tags.length === 0 ? t('imageLibrary.tagsPlaceholder', { defaultValue: 'style, lighting, subject… (Enter to add)' }) : undefined}
                            />
                        )}
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

export default LibraryPromptEditorDialog;
