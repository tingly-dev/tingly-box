import {Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

// Shared by the setup and authorization surfaces, including reauthorization.
export default function WorkBuddyConnectionNote({providerId}: {providerId?: string}) {
    const {t} = useTranslation();
    if (providerId !== 'workbuddy' && providerId !== 'workbuddy_cn') return null;
    return (
        <Typography variant="body2" color="text.secondary">
            {t('workbuddyConnection.scope')}
        </Typography>
    );
}
