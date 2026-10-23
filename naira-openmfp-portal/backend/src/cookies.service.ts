import { Injectable } from '@nestjs/common';
import { AuthTokenData, CookiesService } from '@openmfp/portal-server-lib';
import type { CookieOptions, Request, Response } from 'express';

const authCookie = 'openmfp_auth_cookie';

@Injectable()
export class NairaCookiesService extends CookiesService {
  override setAuthCookie(request: Request, response: Response, tokens: AuthTokenData): void {
    response.cookie(authCookie, tokens.refresh_token, this.cookieOptions(request));
  }

  override removeAuthCookie(request: Request, response: Response): void {
    response.clearCookie(authCookie, this.cookieOptions(request));
  }

  private cookieOptions(request: Request): CookieOptions {
    const localHttp = process.env.ALLOW_INSECURE_LOCALHOST_COOKIES === 'true'
      && request.hostname === 'localhost'
      && request.protocol === 'http';

    return {
      httpOnly: true,
      secure: !localHttp,
      path: '/',
      sameSite: 'strict',
    };
  }
}