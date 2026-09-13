"use client";

import { useEffect, useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { approveMerchantPaymentIntent, getAccounts, getMerchantPaymentIntent } from "@/lib/api";
import { useAuthStore } from "@/lib/store/authStore";
import type { Account, MerchantPaymentIntent } from "@/lib/types";

function iban(value: string) { return value.replace(/(.{4})/g, "$1 ").trim(); }
function eur(value: string) { return new Intl.NumberFormat("de-DE", { style: "currency", currency: "EUR" }).format(Number(value)); }
function apiError(value: unknown) {
	return typeof value === "object" && value !== null && "error" in value ? String((value as { error: unknown }).error) : "Die Zahlung konnte nicht geladen werden.";
}

export default function MerchantPaymentPage() {
	const router = useRouter();
	const params = useParams<{ intentId: string }>();
	const hydrated = useAuthStore((state) => state.isHydrated);
	const authenticated = useAuthStore((state) => state.isAuthenticated());
	const intentID = typeof params.intentId === "string" ? params.intentId : "";
	const [intent, setIntent] = useState<MerchantPaymentIntent | null>(null);
	const [accounts, setAccounts] = useState<Account[]>([]);
	const [sourceAccountID, setSourceAccountID] = useState("");
	const [loading, setLoading] = useState(true);
	const [submitting, setSubmitting] = useState(false);
	const [error, setError] = useState("");
	const source = useMemo(() => accounts.find((account) => account.id === sourceAccountID), [accounts, sourceAccountID]);

	useEffect(() => {
		if (!hydrated) return;
		if (!authenticated) {
			router.replace(`/auth?continue=${encodeURIComponent(`/pay/${intentID}`)}`);
			return;
		}
		let cancelled = false;
		void (async () => {
			try {
				const [intentResult, accountsResult] = await Promise.all([getMerchantPaymentIntent(intentID), getAccounts()]);
				if (!intentResult.response.ok) throw new Error(apiError(intentResult.data));
				if (!accountsResult.response.ok) throw new Error(apiError(accountsResult.data));
				if (cancelled) return;
				setIntent(intentResult.data);
				const eligible = accountsResult.data.filter((account) => account.status === "ACTIVE" && !account.is_system && Number(account.available_balance) > 0);
				setAccounts(eligible);
				setSourceAccountID(eligible.find((account) => account.account_type === "GIROKONTO")?.id || eligible[0]?.id || "");
			} catch (loadError) {
				if (!cancelled) setError(loadError instanceof Error ? loadError.message : "Die Zahlung konnte nicht geladen werden.");
			} finally {
				if (!cancelled) setLoading(false);
			}
		})();
		return () => { cancelled = true; };
	}, [authenticated, hydrated, intentID, router]);

	const approve = async () => {
		if (!intent || !sourceAccountID) return;
		setSubmitting(true); setError("");
		try {
			const result = await approveMerchantPaymentIntent(intent.payment_intent_id, sourceAccountID);
			if (!result.response.ok) throw new Error(apiError(result.data));
			setIntent(result.data);
		} catch (approvalError) {
			setError(approvalError instanceof Error ? approvalError.message : "Die Zahlung konnte nicht freigegeben werden.");
		} finally { setSubmitting(false); }
	};

	if (!hydrated || loading) return <main className="min-h-screen bg-slate-50 p-6"><div className="mx-auto mt-24 max-w-lg rounded-2xl bg-white p-8 text-center text-slate-500 shadow-sm">Zahlungsauftrag wird sicher geladen…</div></main>;
	if (error) return <main className="min-h-screen bg-slate-50 p-6"><section className="mx-auto mt-24 max-w-lg rounded-2xl border border-red-200 bg-white p-8 shadow-sm"><h1 className="text-xl font-bold text-slate-900">Zahlung nicht verfügbar</h1><p className="mt-3 text-sm text-red-700">{error}</p><button onClick={() => router.push("/dashboard")} className="bank-primary mt-6">Zum Online-Banking</button></section></main>;
	if (!intent) return null;
	if (intent.status === "BOOKED") return <main className="min-h-screen bg-slate-50 p-6"><section className="mx-auto mt-24 max-w-lg rounded-2xl border border-emerald-200 bg-white p-8 text-center shadow-sm"><div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-emerald-100 text-2xl text-emerald-700">✓</div><h1 className="mt-5 text-2xl font-bold">Zahlung gebucht</h1><p className="mt-2 text-sm text-slate-500">Ihre Zahlung an {intent.merchant_name} wurde in dieser Demo gebucht.</p><a href={intent.return_url} className="bank-primary mt-6 inline-flex">Zum Shop zurückkehren</a></section></main>;

	return <main className="min-h-screen bg-slate-50 px-4 py-8 md:py-14"><section className="mx-auto max-w-2xl overflow-hidden rounded-[28px] border border-slate-200 bg-white shadow-[0_24px_64px_rgba(15,23,42,.12)]"><header className="bg-gradient-to-r from-[#003b70] to-[#0874b9] px-6 py-7 text-white md:px-9"><p className="text-xs font-bold uppercase tracking-[.18em] text-blue-100">Pehlione DemoBank</p><h1 className="mt-2 text-2xl font-extrabold">Überweisung bestätigen</h1><p className="mt-1 text-sm text-blue-100">Die Zahlungsdaten wurden sicher vom Händler übernommen.</p></header><div className="p-6 md:p-9"><div className="divide-y divide-slate-100 rounded-2xl border border-slate-200">{[[ "Empfänger", intent.merchant_name ], [ "IBAN", iban(intent.beneficiary_iban) ], [ "Betrag", eur(intent.amount) ], [ "Verwendungszweck", intent.merchant_reference ]].map(([label, value]) => <div key={label} className="grid gap-1 px-5 py-4 sm:grid-cols-[180px_1fr]"><span className="text-sm text-slate-500">{label}</span><strong className={label === "IBAN" ? "font-mono text-sm text-slate-800" : "text-slate-900"}>{value}</strong></div>)}</div><p className="mt-3 text-xs text-slate-500">Empfänger, Betrag und Verwendungszweck können aus Sicherheitsgründen nicht geändert werden.</p><label className="mt-7 block"><span className="mb-2 block text-sm font-bold text-slate-800">Von Konto</span><select value={sourceAccountID} onChange={(event) => setSourceAccountID(event.target.value)} className="bank-input"><option value="">Konto auswählen</option>{accounts.map((account) => <option key={account.id} value={account.id}>{account.name} · {account.masked_iban} · {eur(account.available_balance)}</option>)}</select></label>{accounts.length === 0 && <p className="mt-2 text-sm text-red-700">Kein aktives Konto mit verfügbarem Guthaben vorhanden.</p>}<div className="mt-7 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900"><strong>Demo-Zahlung:</strong> Es handelt sich um eine lokale Simulation ohne echte Bankanbindung oder TAN-Verfahren.</div><div className="mt-7 flex flex-wrap justify-between gap-3"><button type="button" onClick={() => router.push("/dashboard")} className="rounded-xl border border-slate-300 px-5 py-3 text-sm font-bold text-slate-700">Abbrechen</button><button type="button" disabled={!source || submitting} onClick={() => void approve()} className="bank-primary disabled:cursor-not-allowed disabled:opacity-50">{submitting ? "Wird gebucht…" : `${eur(intent.amount)} bestätigen`}</button></div></div></section></main>;
}
