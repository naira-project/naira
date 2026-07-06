import { useEffect, useState } from "react";
import * as LuigiClient from "@luigi-project/client";

export interface OpenMFPContext {
  token: string | null;
  userId: string | null;
  userEmail: string | null;
  isReady: boolean;
}

export function useOpenMFPContext(): OpenMFPContext {
  const [ctx, setCtx] = useState<OpenMFPContext>({
    token: null,
    userId: null,
    userEmail: null,
    isReady: false,
  });

  useEffect(() => {
    const update = (context: Record<string, string>) => {
      setCtx({
        token: context.token ?? null,
        userId: context.userId ?? null,
        userEmail: context.userEmail ?? null,
        isReady: true,
      });
    };

    LuigiClient.addInitListener(update);
    LuigiClient.addContextUpdateListener(update);
    
    const timeout = setTimeout(() => {
      setCtx((prev) => {
        if (prev.isReady) {
          return prev;
        }
        return { ...prev, isReady: true };
      });
    }, 500);

    return () => clearTimeout(timeout);
  }, []);

  return ctx;
}
