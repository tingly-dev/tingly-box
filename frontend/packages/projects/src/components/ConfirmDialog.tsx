import { ConfirmDialog as TinglyConfirmDialog } from '@tingly/ui';
import { useTranslation } from 'react-i18next';

interface Props {
    open: boolean;
    title: string;
    message: string;
    confirmLabel: string;
    danger?: boolean;
    onConfirm: () => void;
    onClose: () => void;
}

/** Tingly Box's confirm dialog, with Projects' strings and close-on-confirm. */
export function ConfirmDialog({ open, title, message, confirmLabel, danger, onConfirm, onClose }: Props) {
    const { t } = useTranslation();
    return (
        <TinglyConfirmDialog
            open={open}
            title={title}
            description={message}
            confirmLabel={confirmLabel}
            cancelLabel={t('common.cancel')}
            confirmColor={danger ? 'error' : 'primary'}
            onClose={onClose}
            onConfirm={() => { onConfirm(); onClose(); }}
        />
    );
}
