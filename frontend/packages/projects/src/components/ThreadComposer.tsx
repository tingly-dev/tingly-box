import { useState } from 'react';
import { Composer, type ComposerProps } from './Composer';
import { readLocal, removeLocal, writeLocal } from '../utils/storage';

type Props = Omit<ComposerProps, 'text' | 'onTextChange' | 'onAccepted' | 'keepUntilSent' | 'sendWhileStreaming'> & {
    /** Names the draft: what is typed but not sent survives leaving the page or a reload. */
    draftKey: string;
};

/**
 * The message box of a conversation with an agent, a project's own or a
 * thread's: the same draft, the same "send while a turn runs" and the same
 * rules for when the text leaves the box, so every surface behaves alike.
 * Mount it with a key per conversation so its draft starts fresh.
 */
export function ThreadComposer({ draftKey, ...rest }: Props) {
    const key = `draft.${draftKey}`;
    const [draft, setDraftState] = useState(() => readLocal(key) ?? '');
    const setDraft = (text: string) => {
        setDraftState(text);
        if (text) writeLocal(key, text);
        else removeLocal(key);
    };
    return (
        <Composer
            {...rest}
            keepUntilSent
            sendWhileStreaming
            text={draft}
            onTextChange={setDraft}
            // Clear only what was sent: typing may have gone on meanwhile.
            onAccepted={(sent) => setDraftState((prev) => {
                if (prev !== sent) return prev;
                removeLocal(key);
                return '';
            })}
        />
    );
}
