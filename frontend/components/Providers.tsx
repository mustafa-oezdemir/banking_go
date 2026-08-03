/**
 * Providers Component
 * Client-side providers for state hydration and initialization
 */

"use client";

import { useEffect } from "react";
import { useAuthStore } from "@/lib/store/authStore";
import { clearPageRecoveryMarker } from "@/lib/pageRecovery";

export function Providers({ children }: { children: React.ReactNode }) {
  const hydrate = useAuthStore((state) => state.hydrate);

  useEffect(() => {
    // Listen for auth logout events (e.g., from 401 responses)
    const handleLogout = () => {
      useAuthStore.getState().logout();
    };

    window.addEventListener("auth:logout", handleLogout);
    // Validate the HttpOnly cookie session before hydrating client state.
    void hydrate();
    const recoveryTimer = window.setTimeout(clearPageRecoveryMarker, 10_000);
    return () => {
      window.removeEventListener("auth:logout", handleLogout);
      window.clearTimeout(recoveryTimer);
    };
  }, [hydrate]);

  // Always render children - hydration happens in useEffect above
  // The page component will handle displaying loading state if needed
  return <>{children}</>;
}
