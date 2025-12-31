import React from 'react';
import PageLayout from '../../components/PageLayout';
import UseOpenAIPage from '../UseOpenAIPage';
import { useFunctionPanelData } from '../../hooks/useFunctionPanelData';

const UseOpenAIPageWrapper: React.FC = () => {
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
        loading,
        notification,
    } = useFunctionPanelData();

    return (
        <PageLayout notification={notification} loading={loading}>
            <UseOpenAIPage
                showTokenModal={showTokenModal}
                setShowTokenModal={setShowTokenModal}
                token={token}
                showNotification={showNotification}
                providers={providers}
            />
        </PageLayout>
    );
};

export default UseOpenAIPageWrapper;
