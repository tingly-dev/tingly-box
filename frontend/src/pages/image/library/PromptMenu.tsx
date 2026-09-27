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
import { usePieceKindLabel } from './fields';
import { findPromptByText, pieceLabel, type PromptPiece } from './model';

// How many of each the menu lists; the rest are on the library page, which
// has search and tags. A menu that scrolls is a worse library.
const MENU_ITEMS = 6;

interface PromptMenuProps {
    prompt: string;
    pieces: PromptPiece[];
    onSave: () => void;
    // A whole prompt replaces the field; a term or phrase is added to it.
    onReplace: (text: string) => void;
    onAppend: (text: string) => void;
}

// One button on the prompt field for both directions — keep this prompt, or
// bring kept material back — so the field does not grow a button per action.
// Whole prompts and pieces are listed apart because they do different things
// to the field. This is prompt assembly by hand.
const PromptMenu: React.FC<PromptMenuProps> = ({ prompt, pieces, onSave, onReplace, onAppend }) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const kindLabel = usePieceKindLabel();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const close = () => setAnchor(null);
    const label = t('imageLibrary.promptMenu', { defaultValue: 'Saved prompts' });
    const alreadySaved = Boolean(prompt.trim()) && findPromptByText(pieces, prompt) !== undefined;
    const prompts = pieces.filter((piece) => piece.kind === 'prompt');
    const parts = pieces.filter((piece) => piece.kind !== 'prompt');

    const section = (heading: string, items: PromptPiece[], onPick: (text: string) => void) => items.length > 0 && [
        <ListSubheader key={heading} sx={{ lineHeight: '32px', bgcolor: 'transparent' }}>{heading}</ListSubheader>,
        ...items.slice(0, MENU_ITEMS).map((piece) => (
            <MenuItem key={piece.id} onClick={() => { onPick(piece.text); close(); }}>
                <ListItemText
                    primary={piece.kind === 'prompt' ? pieceLabel(piece) : piece.text}
                    secondary={piece.kind === 'prompt'
                        ? (piece.title.trim() ? piece.text : undefined)
                        : [kindLabel(piece.kind), ...piece.tags].join(' · ')}
                    slotProps={{
                        primary: { noWrap: true, variant: 'body2' },
                        secondary: { noWrap: true, variant: 'caption' },
                    }}
                />
            </MenuItem>
        )),
    ];

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
                <MenuItem disabled={!prompt.trim() || alreadySaved} onClick={() => { onSave(); close(); }}>
                    <ListItemIcon>{alreadySaved ? <Check fontSize="small" /> : <BookmarkAdd fontSize="small" />}</ListItemIcon>
                    <ListItemText>
                        {alreadySaved
                            ? t('imageLibrary.promptAlreadySaved', { defaultValue: 'This prompt is in the library' })
                            : t('imageLibrary.savePrompt', { defaultValue: 'Save this prompt' })}
                    </ListItemText>
                </MenuItem>
                <Divider />
                {pieces.length === 0 && (
                    <MenuItem disabled>
                        <ListItemText slotProps={{ primary: { variant: 'body2' } }}>
                            {t('imageLibrary.noPrompts', { defaultValue: 'Nothing saved yet' })}
                        </ListItemText>
                    </MenuItem>
                )}
                {section(t('imageLibrary.appendHeading', { defaultValue: 'Add to the prompt' }), parts, onAppend)}
                {section(t('imageLibrary.loadPromptHeading', { defaultValue: 'Replace the prompt with' }), prompts, onReplace)}
                <Divider />
                <MenuItem onClick={() => { close(); navigate('/image/library'); }}>
                    <ListItemIcon><PhotoLibrary fontSize="small" /></ListItemIcon>
                    <ListItemText>
                        {prompts.length > MENU_ITEMS || parts.length > MENU_ITEMS
                            ? t('imageLibrary.openLibraryAll', { defaultValue: 'All {{count}} in the library', count: pieces.length })
                            : t('imageLibrary.openLibrary', { defaultValue: 'Open the library' })}
                    </ListItemText>
                </MenuItem>
            </Menu>
        </>
    );
};

export default PromptMenu;
