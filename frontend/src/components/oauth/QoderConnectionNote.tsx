import {Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

// Shown during setup and authorization, including reauthorization.
export default function QoderConnectionNote({providerId}: {providerId?: string}) {
    const {t} = useTranslation();
    if (providerId !== 'qoder' && providerId !== 'qoder_cn') return null;
    return (
        <Typography variant="body2" color="text.secondary">
            {t('qoderConnection.scope')}
        </Typography>
    );
}
