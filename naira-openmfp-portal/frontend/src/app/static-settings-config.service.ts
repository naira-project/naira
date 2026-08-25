import { Injectable } from '@angular/core';
import { LuigiStaticSettings, StaticSettingsConfigService } from '@openmfp/portal-ui-lib';

@Injectable({ providedIn: 'root' })
export class NairaStaticSettingsConfigService implements StaticSettingsConfigService {
  async getStaticSettingsConfig(): Promise<LuigiStaticSettings> {
    return {
      header: {
        title: 'Naira',
        logo: 'assets/images/naira.png',
        favicon: 'assets/images/naira-favicon.ico',
      },
    };
  }
}
