import { useTranslation } from 'react-i18next';
import CopyIconButton from '@tingly/ui/components/CopyIconButton';

interface ModelCopyButtonProps {
    model: string;
}

// ModelCopyButton: copies the exact model id from a card's hover-only
// ControlBar. Sized to sit beside the other 14px control-bar actions.
export function ModelCopyButton({ model }: ModelCopyButtonProps) {
    const { t } = useTranslation();
    return (
        <CopyIconButton
            value={model}
            label={t('common.copyModelName')}
            copiedLabel={t('common.modelNameCopied')}
            iconSize={14}
            sx={{
                p: 0.3,
                '&:hover': { backgroundColor: 'action.hover', color: 'primary.main' },
            }}
        />
    );
}

export default ModelCopyButton;
