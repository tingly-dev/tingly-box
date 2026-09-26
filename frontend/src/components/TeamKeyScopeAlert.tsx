import {Alert, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import type {Team} from '@/types/team';

interface TeamKeyScopeAlertProps {
    team: Team;
}

const TeamKeyScopeAlert: React.FC<TeamKeyScopeAlertProps> = ({team}) => {
    const {t} = useTranslation();

    return (
        <Alert
            severity="info"
            variant="outlined"
            sx={{
                py: 0.25,
                alignItems: 'center',
                '& .MuiAlert-message': {py: 0.5},
            }}
        >
            <Typography variant="body2">
                {t('teams.keyScopeSummary', {team: team.name, slug: team.slug})}
            </Typography>
        </Alert>
    );
};

export default TeamKeyScopeAlert;
