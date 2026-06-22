import { inject, provideAppInitializer, provideZonelessChangeDetection } from "@angular/core";
import { bootstrapApplication } from "@angular/platform-browser";
import { PortalComponent, PortalOptions, providePortal, EnvConfigService, LocalStorageKeys } from "@openmfp/portal-ui-lib";

const portalOptions: PortalOptions = {};

bootstrapApplication(PortalComponent, {
  providers: [
    provideZonelessChangeDetection(),
    providePortal(portalOptions),
    provideAppInitializer(async () => {
      const envConfigService = inject(EnvConfigService);
      const env = await envConfigService.getEnvConfig();
      if (!env.developmentInstance) return;

      const key = LocalStorageKeys.LOCAL_DEVELOPMENT_SETTINGS;
      if (localStorage.getItem(key)) return;

      localStorage.setItem(key, JSON.stringify({ isActive: true, configs: [], serviceProviderConfig: {} }));
    }),
  ],
}).catch((err) => console.error(err));
