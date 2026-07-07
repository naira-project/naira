import { useNavigate, useLocation } from 'react-router';
import {
  LayoutDashboard,
  Layers,
  Plus,
  Settings,
  HelpCircle,
} from 'lucide-react';
import { Separator } from './ui/separator';
import { NAV_CATALOG } from '../constants/Constants';
import { cn } from '../lib/utils';

const NAV_PRIMARY = [
  { label: 'Dashboard', icon: LayoutDashboard },
  { label: 'Model Registries', icon: Layers },
  { label: 'MCP Catalog', icon: Plus },
];

const NAV_BOTTOM = [
  { label: 'Settings', icon: Settings },
  { label: 'Help & Support', icon: HelpCircle },
];

const routeMap: Record<string, string> = {
  Dashboard: '/',
  'Model Registries': '/model-registry',
  'MCP Catalog': '/',
  Datasets: '/dataset-registry',
};

export default function NavigationBar() {
  const navigate = useNavigate();
  const location = useLocation();

  const activeNav =
    location.pathname === '/'
      ? 'Dashboard'
      : location.pathname === '/model-registry'
      ? 'Model Registries'
      : location.pathname === '/dataset-registry'
      ? 'Datasets'
      : location.pathname === '/catalog-graph'
      ? 'Catalog Graph'
      : 'Dashboard';

  return (
    <nav className="flex w-[25%] min-w-[220px] max-w-[300px] flex-shrink-0 flex-col overflow-y-auto bg-primary-dark scrollbar-thin">
      {/* Logo */}
      <div className="flex items-center gap-3 px-5 py-5">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-secondary-light to-primary-light">
          <img src="/logo.png" alt="Logo" className="h-[50px] w-[50px]" />
        </div>
        <span className="text-base font-bold tracking-wide text-[#e0f7fa]">Naira</span>
      </div>

      <Separator className="mx-4" />

      {/* Primary nav */}
      <div className="px-3 pt-4">
        <p className="px-3 text-[0.65rem] font-semibold uppercase tracking-widest text-white/80">Assets</p>
        <ul className="mt-1 space-y-1">
          {NAV_PRIMARY.map(({ label, icon: Icon }) => {
            const active = activeNav === label;
            return (
              <li key={label}>
                <button
                  onClick={() => navigate(routeMap[label] ?? '/')}
                  className={cn(
                    'flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors',
                    active
                      ? 'bg-white/15 font-semibold text-white'
                      : 'font-normal text-white hover:bg-white/10 hover:text-white'
                  )}
                >
                  <Icon size={18} className="shrink-0" />
                  {label}
                </button>
              </li>
            );
          })}
          {NAV_CATALOG.map((label) => {
            const active = activeNav === label;
            return (
              <li key={label}>
                <button
                  onClick={() => navigate(routeMap[label] ?? '/')}
                  className={cn(
                    'flex w-full items-center rounded-lg px-3 py-2 pl-10 text-sm transition-colors',
                    active
                      ? 'bg-white/15 font-semibold text-white'
                      : 'text-white hover:bg-white/10 hover:text-white'
                  )}
                >
                  {label}
                </button>
              </li>
            );
          })}
        </ul>
      </div>

      <div className="flex-1" />

      <Separator className="mx-4" />

      {/* Bottom nav */}
      <div className="px-3 py-3">
        <ul className="space-y-1">
          {NAV_BOTTOM.map(({ label, icon: Icon }) => (
            <li key={label}>
              <button className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-sm text-white hover:bg-white/10 hover:text-white">
                <Icon size={18} className="shrink-0" />
                {label}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </nav>
  );
}
