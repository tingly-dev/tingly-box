import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const FeishuPage = () => {
    const config = getPlatformGuide('feishu');

    return (
        <PlatformBotPage
            platformId="feishu"
            platformName={config?.name || 'Feishu (飞书)'}
            platformGuide={config?.guide}
        />
    );
};

export default FeishuPage;
