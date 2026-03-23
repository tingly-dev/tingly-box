import PlatformBotPage from './PlatformBotPage';
import { getPlatformGuide } from '@/constants/platformGuides';

const DingTalkPage = () => {
    const config = getPlatformGuide('dingtalk');

    return (
        <PlatformBotPage
            platformId="dingtalk"
            platformName={config?.name || 'DingTalk (钉钉)'}
            platformGuide={config?.guide}
        />
    );
};

export default DingTalkPage;
