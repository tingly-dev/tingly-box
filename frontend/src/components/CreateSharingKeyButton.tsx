import { Add as IconPlus } from '@/components/icons';
import { Button, Tooltip, type ButtonProps } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Team } from '@/types/team';

interface CreateSharingKeyButtonProps {
    team: Team;
    onClick: () => void;
    variant?: ButtonProps['variant'];
    size?: ButtonProps['size'];
}

/**
 * "Create Token" for one Team. Keys can only be created in an enabled Team,
 * so for a disabled Team the button stays visible but disabled, with the
 * reason on hover instead of failing after the user fills in the dialog.
 */
const CreateSharingKeyButton: React.FC<CreateSharingKeyButtonProps> = ({ team, onClick, variant = 'contained', size }) => {
    const { t } = useTranslation();
    const button = (
        <Button
            variant={variant}
            size={size}
            startIcon={<IconPlus sx={{ fontSize: 18 }} />}
            onClick={onClick}
            disabled={!team.enabled}
        >
            {t('sharingKeys.createToken')}
        </Button>
    );
    // An empty title renders no tooltip, so enabled Teams get a plain button.
    return (
        <Tooltip title={team.enabled ? '' : t('sharingKeys.createDisabledTeam')}>
            {/* span keeps the tooltip working on a disabled button */}
            <span>{button}</span>
        </Tooltip>
    );
};

export default CreateSharingKeyButton;
