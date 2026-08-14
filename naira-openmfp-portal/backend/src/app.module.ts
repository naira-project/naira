import { Module } from '@nestjs/common';
import {
  EnvAuthConfigService,
  PortalModule,
  PortalModuleOptions,
} from '@openmfp/portal-server-lib';
import { config } from 'dotenv';
import * as path from 'node:path';
import { ServiceProviderServiceImpl } from './service-provider.js';

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
};

@Module({
  imports: [PortalModule.create(portalOptions)],
})
export class AppModule {}
