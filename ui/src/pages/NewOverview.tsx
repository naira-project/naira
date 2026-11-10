import { WelcomeBannerIllustration } from '../components/overview/WelcomeBannerIllustration';
import { WelcomeBannerIntro } from '../components/overview/WelcomeBannerIntro';
import { WelcomeBannerTagline } from '../components/overview/WelcomeBannerTagline';

export default function NewOverview() {
  return (
    <div className="p-4">
      <section className="flex items-center gap-5 self-stretch rounded-[10px] bg-solitude">
        <WelcomeBannerIntro />
        <WelcomeBannerIllustration />
        <WelcomeBannerTagline />
      </section>
    </div>
  );
}
