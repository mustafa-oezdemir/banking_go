"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@/lib/store/authStore";

export default function Page() {
  const router = useRouter();
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated());
  const isHydrated = useAuthStore((state) => state.isHydrated);

  useEffect(() => {
    if (isHydrated) {
      router.push(isAuthenticated ? "/dashboard" : "/auth");
    }
  }, [isHydrated, isAuthenticated, router]);

  return (
		<div className="min-h-screen bg-[#f3f5f7] flex items-center justify-center">
      <div className="text-center">
        <div className="spinner mx-auto mb-4"></div>
				<p className="text-slate-500">Demo-Banking wird geladen…</p>
      </div>
    </div>
  );
}
