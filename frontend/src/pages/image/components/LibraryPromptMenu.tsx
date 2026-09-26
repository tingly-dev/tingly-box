import { useState } from 'react';
import {
    Divider,
    IconButton,
    ListItemIcon,
    ListItemText,
    ListSubheader,
    Menu,
    MenuItem,
    Tooltip,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Bookmark, BookmarkAdd, Check, PhotoLibrary } from '@/components/icons';
import { findPromptByText, promptLabel, type LibraryPrompt } from '@/utils/imageLibrary';

// How many of each the menu lists. The rest are one click away on the
// library page, which has search and tags; a menu that scrolls is a worse
// library.
const MENU_ITEMS = 6;

interface LibraryPromptMenuProps {
    prompt: string;
    prompts: LibraryPrompt[];
    onSave: () => void;
    // A whole prompt replaces the field.
    onLoad: (prompt: LibraryPrompt) => void;
    // A term or phrase is added to what is already written.
    onAppend: (piece: LibraryPrompt) => void;
}

// One button on the prompt field for both directions — keep this prompt, or
// bring kept material back — so the field's adornment does not grow a button
// per action. Whole prompts and pieces are listed apart because they do
// different things to the field: one replaces it, the other adds to it. This
// is prompt assembly by hand; see .design/image-library.md.
const LibraryPromptMenu: React.FC<LibraryPromptMenuProps> = ({ prompt, prompts, onSave, onLoad, onAppend }) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const label = t('imageLibrary.promptMenu', { defaultValue: 'Saved prompts' });
    const alreadySaved = prompt.trim() ? findPromptByText(prompts, prompt) !== undefined : false;
    const close = () => setAnchor(null);
    const wholePrompts = prompts.filter((item) => item.kind === 'prompt');
    const pieces = prompts.filter((item) => item.kind !== 'prompt');
    const kindLabel = (item: LibraryPrompt) => (item.kind === 'term'
        ? t('imageLibrary.kind.term', { defaultValue: 'Term' })
        : t('imageLibrary.kind.phrase', { defaultValue: 'Phrase' }));
    return (
        <>
            <Tooltip title={label}>
                <IconButton size="small" onClick={(event) => setAnchor(event.currentTarget)} aria-label={label}>
                    <Bookmark sx={{ fontSize: 16 }} />
                </IconButton>
            </Tooltip>
            <Menu
                anchorEl={anchor}
                open={anchor !== null}
                onClose={close}
                slotProps={{ paper: { sx: { width: 340, maxWidth: 'calc(100vw - 32px)' } } }}
            >
                <MenuItem
                    disabled={!prompt.trim() || alreadySaved}
                    onClick={() => { onSave(); close(); }}
                >
                    <ListItemIcon>
                        {alreadySaved ? <Check fontSize="small" /> : <BookmarkAdd fontSize="small" />}
                    </ListItemIcon>
                    <ListItemText>
                        {alreadySaved
                            ? t('imageLibrary.promptAlreadySaved', { defaultValue: 'This prompt is in the library' })
                            : t('imageLibrary.savePrompt', { defaultValue: 'Save this prompt' })}
                    </ListItemText>
                </MenuItem>
                <Divider />
                {prompts.length === 0 && (
                    <MenuItem disabled>
                        <ListItemText
                            primary={t('imageLibrary.noPrompts', { defaultValue: 'Nothing saved yet' })}
                            slotProps={{ primary: { variant: 'body2' } }}
                        />
                    </MenuItem>
                )}
                {pieces.length > 0 && (
                    <ListSubheader sx={{ lineHeight: '32px', bgcolor: 'transparent' }}>
                        {t('imageLibrary.appendHeading', { defaultValue: 'Add to the prompt' })}
                    </ListSubheader>
                )}
                {pieces.slice(0, MENU_ITEMS).map((item) => (
                    <MenuItem key={item.id} onClick={() => { onAppend(item); close(); }}>
                        <ListItemText
                            primary={item.text}
                            secondary={[kindLabel(item), ...item.tags].join(' · ')}
                            slotProps={{
                                primary: { noWrap: true, variant: 'body2' },
                                secondary: { noWrap: true, variant: 'caption' },
                            }}
                        />
                    </MenuItem>
                ))}
                {wholePrompts.length > 0 && (
                    <ListSubheader sx={{ lineHeight: '32px', bgcolor: 'transparent' }}>
                        {t('imageLibrary.loadPromptHeading', { defaultValue: 'Replace the prompt with' })}
                    </ListSubheader>
                )}
                {wholePrompts.slice(0, MENU_ITEMS).map((item) => (
                    <MenuItem key={item.id} onClick={() => { onLoad(item); close(); }}>
                        <ListItemText
                            primary={promptLabel(item)}
                            secondary={item.title.trim() ? item.text : undefined}
                            slotProps={{
                                primary: { noWrap: true, variant: 'body2' },
                                secondary: { noWrap: true, variant: 'caption' },
                            }}
                        />
                    </MenuItem>
                ))}
                <Divider />
                <MenuItem onClick={() => { close(); navigate('/image/library'); }}>
                    <ListItemIcon><PhotoLibrary fontSize="small" /></ListItemIcon>
                    <ListItemText>
                        {wholePrompts.length > MENU_ITEMS || pieces.length > MENU_ITEMS
                            ? t('imageLibrary.openLibraryAll', { defaultValue: 'All {{count}} in the library', count: prompts.length })
                            : t('imageLibrary.openLibrary', { defaultValue: 'Open the library' })}
                    </ListItemText>
                </MenuItem>
            </Menu>
        </>
    );
};

export default LibraryPromptMenu;
