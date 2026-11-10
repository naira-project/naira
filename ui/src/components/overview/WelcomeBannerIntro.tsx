import { Search } from 'lucide-react';
import { Input } from '../ui/input';

export function WelcomeBannerIntro() {
  return (
    <div className="flex h-[199px] w-[524px] flex-col items-start gap-[22px] px-[26px] py-5">
      <div className="flex flex-col gap-1.5">
        <p className="flex w-[172px] flex-1 flex-col justify-center text-xl font-bold text-deep-sapphire">
          Welcome to Naira
        </p>
        <h1 className="w-[367px] text-[32px] font-bold text-deep-sapphire">Your AI Hub Platform</h1>
        <p className="w-[570px] text-base font-medium text-deep-sapphire">
          Discover, manage and connect your models, data and tools.
        </p>
      </div>
      <Input
        type="search"
        placeholder="Search assets, projects...."
        aria-label="Search assets and projects"
        startAdornment={<Search className="size-[18px] shrink-0 text-[#1769E8]" />}
        className="h-[38px] w-[379px] shrink-0 rounded-[4px] border-[0.5px] border-alice-blue bg-white py-1 pr-2.5 pl-[46px] text-xs leading-6 shadow-none placeholder:text-[rgba(152,166,187,0.75)] md:text-xs"
      />
    </div>
  );
}
