import { Box } from '@mui/material';
import ConfirmDialog from '@/components/ConfirmDialog';
import { type SkillLocation } from '@/types/prompt';

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
                                fontFamily: 'monospace',
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
