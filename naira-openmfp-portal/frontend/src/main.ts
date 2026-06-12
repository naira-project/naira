import { provideZonelessChangeDetection } from "@angular/core";
import { bootstrapApplication } from "@angular/platform-browser";
import { PortalComponent, PortalOptions, providePortal } from "@openmfp/portal-ui-lib";

const portalOptions: PortalOptions = {};

bootstrapApplication(PortalComponent, {
  providers: [provideZonelessChangeDetection(), providePortal(portalOptions)],
}).catch((err) => console.error(err));
