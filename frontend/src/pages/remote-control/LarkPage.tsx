import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const LarkPage = () => {
    const config = getPlatformGuide('lark');

    return (
        <PlatformBotPage
            platformId="lark"
            platformName={config?.name || 'Lark'}
            platformGuide={config?.guide}
        />
    );
};

export default LarkPage;
