import {ContentCopy, ExpandMore} from '@/components/icons';
import {
    Button,
    CircularProgress,
    Menu,
    MenuItem,
} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    exportProviderAsBase64ToClipboard,
    exportProviderAsJsonlToClipboard,
    type ExportFormat,
} from '@/components/rule-card/utils';

interface ProviderExportButtonProps {
    providerUuid: string;
    onNotification?: (message: string, severity: 'success' | 'error') => void;
}

/**
 * Compact access to the same provider clipboard exports offered by the
 * credentials-table overflow menu. Intended for provider edit surfaces.
 */
const ProviderExportButton = ({providerUuid, onNotification}: ProviderExportButtonProps) => {
    const {t} = useTranslation();
    const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
    const [copying, setCopying] = useState<ExportFormat | null>(null);

    const handleCopy = async (format: ExportFormat) => {
        setAnchorEl(null);
        setCopying(format);
        const notify = onNotification ?? (() => undefined);
        const provider = {uuid: providerUuid};
        try {
            if (format === 'base64') {
                await exportProviderAsBase64ToClipboard(provider, notify);
            } else {
                await exportProviderAsJsonlToClipboard(provider, notify);
            }
        } finally {
            setCopying(null);
        }
    };

    return (
        <>
            <Button
                type="button"
                variant="text"
                size="small"
                color="inherit"
                startIcon={copying ? <CircularProgress size={15}/> : <ContentCopy fontSize="small"/>}
                endIcon={<ExpandMore fontSize="small"/>}
                disabled={copying !== null}
                onClick={(event) => setAnchorEl(event.currentTarget)}
                aria-haspopup="menu"
                aria-expanded={Boolean(anchorEl)}
            >
                {copying
                    ? t('providerDialog.export.copying')
                    : t('providerDialog.export.action')}
            </Button>
            <Menu
                anchorEl={anchorEl}
                open={Boolean(anchorEl)}
                onClose={() => setAnchorEl(null)}
                anchorOrigin={{vertical: 'top', horizontal: 'left'}}
                transformOrigin={{vertical: 'bottom', horizontal: 'left'}}
            >
                <MenuItem onClick={() => handleCopy('base64')}>
                    <ContentCopy fontSize="small" sx={{mr: 1}}/>
                    {t('providerDialog.export.base64')}
                </MenuItem>
                <MenuItem onClick={() => handleCopy('jsonl')}>
                    <ContentCopy fontSize="small" sx={{mr: 1}}/>
                    {t('providerDialog.export.jsonl')}
                </MenuItem>
            </Menu>
        </>
    );
};

export default ProviderExportButton;
