import { Button } from '@mui/material';
import UnifiedCard from './UnifiedCard';
import { api } from '../services/api';

const AuthenticationCard = () => {
    const handleGenerateToken = async () => {
        const clientId = prompt('Enter client ID (web):', 'web');
        if (clientId) {
            const result = await api.generateToken(clientId);
            if (result.success) {
                navigator.clipboard.writeText(result.data.token);
                alert('Token generated and copied to clipboard!');
            } else {
                alert(result.error);
            }
        }
    };

    return (
        <UnifiedCard
            title="Authentication"
            subtitle="Generate JWT token for API access"
            size="medium"
        >
            <Button variant="contained" onClick={handleGenerateToken}>
                Generate Token
            </Button>
        </UnifiedCard>
    );
};

export default AuthenticationCard;
