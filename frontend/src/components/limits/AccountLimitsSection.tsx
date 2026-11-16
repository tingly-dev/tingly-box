import {Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {ProviderFormSection} from '@/components/provider-form-dialog/ProviderFormLayout';
import LimitsEditor, {type Limits} from './LimitsEditor';
import LimitsLiveStatus from './LimitsLiveStatus';
import {useLimitsStatus} from './useLimitsStatus';

interface AccountLimitsSectionProps {
    providerUuid?: string;
    value?: Limits;
    onChange: (next: Limits) => void;
}

/** Account-level (Provider.Limits) section of the provider edit dialogs. */
export default function AccountLimitsSection({providerUuid, value, onChange}: AccountLimitsSectionProps) {
    const {t} = useTranslation();
    const status = useLimitsStatus(!!providerUuid);
    const entry = status.find((s) => s.key === `acct:${providerUuid}`);
    return (
        <ProviderFormSection title={t('limits.title')}>
            <Stack spacing={1.5}>
                <Typography variant="body2" color="text.secondary">{t('limits.hintAccount')}</Typography>
                <LimitsLiveStatus entry={entry}/>
                <LimitsEditor value={value} onChange={onChange} showQuota/>
            </Stack>
        </ProviderFormSection>
    );
}
