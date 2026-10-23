import { Module } from '@nestjs/common';
import {
  CookiesService,
  EnvAuthConfigService,
  PortalModule,
  PortalModuleOptions,
} from '@openmfp/portal-server-lib';
import { config } from 'dotenv';
import * as path from 'node:path';
import { ServiceProviderServiceImpl } from './service-provider.js';
import { NairaCookiesService } from './cookies.service.js';

const __filename = new URL(import.meta.url).pathname;
const __dirname = path.dirname(__filename);

config({ path: './.env' });

const portalOptions: PortalModuleOptions = {
  frontendDistSources: path.join(
    __dirname,
    '../..',
    'frontend/dist/frontend/browser',
  ),
  serviceProviderService: ServiceProviderServiceImpl,
  authConfigProvider: EnvAuthConfigService,
  additionalProviders: [{ provide: CookiesService, useClass: NairaCookiesService }],
};

@Module({
  imports: [PortalModule.create(portalOptions)],
})
export class AppModule {}
