import { Box } from '@mui/material';
import ConfirmDialog from '@tingly/ui/components/ConfirmDialog';
import { type SkillLocation } from '@/types/prompt';
import { fontMono } from '@tingly/ui/theme/fonts';

interface DeleteSkillLocationDialogProps {
    location: SkillLocation | null;
    loading?: boolean;
    onClose: () => void;
    onConfirm: () => void;
}

const DeleteSkillLocationDialog = ({
    location,
    loading = false,
    onClose,
    onConfirm,
}: DeleteSkillLocationDialogProps) => {
    return (
        <ConfirmDialog
            open={Boolean(location)}
            title="Delete skill location?"
            description={
                location ? (
                    <>
                        Remove <strong>{location.name}</strong> from Tingly Box?
                        <Box
                            component="span"
                            sx={{
                                display: 'block',
                                mt: 1,
                                fontFamily: fontMono,
                                overflowWrap: 'anywhere',
                            }}
                        >
                            {location.path}
                        </Box>
                        <Box component="span" sx={{ display: 'block', mt: 1 }}>
                            Files on disk will not be deleted.
                        </Box>
                    </>
                ) : undefined
            }
            confirmLabel="Delete location"
            confirmingLabel="Deleting…"
            confirmColor="error"
            loading={loading}
            onClose={onClose}
            onConfirm={onConfirm}
        />
    );
};

export default DeleteSkillLocationDialog;
