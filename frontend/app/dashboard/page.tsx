"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { BankingApp } from "@/components/banking/BankingApp";
import { useAuthStore } from "@/lib/store/authStore";

export default function DashboardPage() {
	const router = useRouter();
	const authenticated = useAuthStore((state) => state.isAuthenticated());
	const hydrated = useAuthStore((state) => state.isHydrated);

	useEffect(() => {
		if (hydrated && !authenticated) router.replace("/auth");
	}, [authenticated, hydrated, router]);

	if (!hydrated || !authenticated) return <div className="min-h-screen bg-[#f3f5f7]" />;
	return <BankingApp />;
}
