import {Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {AdvancedSection} from '@/components/AdvancedSection';
import {ProviderFormSection} from '@/components/provider-form-dialog/ProviderFormLayout';
import LimitsEditor, {type Limits} from './LimitsEditor';
import LimitsLiveStatus from './LimitsLiveStatus';
import {countLimits} from './limitsSummary';
import {useLimitsStatus} from './useLimitsStatus';

interface AccountLimitsSectionProps {
    providerUuid?: string;
    value?: Limits;
    onChange: (next: Limits) => void;
}

/**
 * Account-level (Provider.Limits) section of the provider edit dialogs. Most
 * accounts are never capped here (a rule's share is set on its node), so it
 * sits behind the shared Advanced disclosure, closed by default; set caps show
 * as the "N customised" badge on the closed header. The live readout only
 * polls while it is open.
 */
export default function AccountLimitsSection({providerUuid, value, onChange}: AccountLimitsSectionProps) {
    return (
        <AdvancedSection customizedCount={countLimits(value)} unmountWhenClosed>
            <Body providerUuid={providerUuid} value={value} onChange={onChange}/>
        </AdvancedSection>
    );
}

function Body({providerUuid, value, onChange}: AccountLimitsSectionProps) {
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
