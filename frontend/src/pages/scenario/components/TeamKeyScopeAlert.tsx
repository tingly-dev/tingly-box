import {Alert, AlertTitle, Chip, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import type {Team} from '@/types/team';

interface TeamKeyScopeAlertProps {
    team: Team;
}

const TeamKeyScopeAlert: React.FC<TeamKeyScopeAlertProps> = ({team}) => {
    const {t} = useTranslation();

    return (
        <Alert severity="info" variant="outlined">
            <AlertTitle>{t('teams.keyScopeTitle')}</AlertTitle>
            <Stack spacing={0.75}>
                <Typography variant="body2">
                    {t('teams.keyScopeOwner', {team: team.name, slug: team.slug})}
                </Typography>
                <Stack direction="row" spacing={1} sx={{alignItems: 'center', flexWrap: 'wrap'}}>
                    <Typography variant="body2">{t('teams.keyScopeEndpoint')}</Typography>
                    <Chip size="small" label="/tingly/team" sx={{fontFamily: 'monospace'}} />
                    <Chip size="small" label="/tingly/team/v1" sx={{fontFamily: 'monospace'}} />
                </Stack>
                <Typography variant="body2">
                    {t('teams.keyScopeRestriction')}
                </Typography>
            </Stack>
        </Alert>
    );
};

export default TeamKeyScopeAlert;
