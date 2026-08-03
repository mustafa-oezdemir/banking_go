"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { resetPassword } from "@/lib/api";

function ResetPasswordForm() {
	const searchParams = useSearchParams();
	const token = searchParams.get("token") || "";
	const [loading, setLoading] = useState(false);
	const [success, setSuccess] = useState(false);
	const [message, setMessage] = useState(token ? "" : "Der Reset-Link ist unvollständig.");

	const submit = async (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		setMessage("");
		const data = new FormData(event.currentTarget);
		const password = String(data.get("password") || "");
		const confirmation = String(data.get("confirmation") || "");
		if (password !== confirmation) {
			setMessage("Die Passwörter stimmen nicht überein.");
			return;
		}
		setLoading(true);
		try {
			const result = await resetPassword(token, password);
			if (!result.response.ok) {
				const apiError = typeof result.data === "object" && result.data !== null && "error" in result.data
					? String((result.data as { error: unknown }).error)
					: "Der Link ist ungültig oder abgelaufen.";
				throw new Error(apiError);
			}
			setSuccess(true);
			setMessage("Ihr Passwort wurde geändert. Alle bisherigen Sitzungen wurden beendet.");
		} catch (error) {
			setMessage(error instanceof Error ? error.message : "Das Passwort konnte nicht geändert werden.");
		} finally {
			setLoading(false);
		}
	};

	return <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-7 shadow-xl md:p-9">
		<div className="mb-7 flex items-center gap-3"><div className="flex h-10 w-10 items-center justify-center rounded-lg bg-[#003b70] font-bold text-white">P</div><div><h1 className="font-bold text-[#003b70]">Pehlione DemoBank</h1><p className="text-xs text-slate-500">Sicherheitscenter</p></div></div>
		<h2 className="text-2xl font-bold">Neues Passwort festlegen</h2>
		<p className="mt-2 text-sm text-slate-500">Der Link ist 15 Minuten gültig und kann nur einmal verwendet werden.</p>
		{!success && <form onSubmit={submit} className="mt-6 space-y-5">
			<label className="block"><span className="mb-2 block text-sm font-semibold">Neues Passwort</span><input name="password" required type="password" minLength={15} maxLength={72} autoComplete="new-password" className="bank-input" /><span className="mt-1 block text-xs text-slate-400">Mindestens 15 Zeichen.</span></label>
			<label className="block"><span className="mb-2 block text-sm font-semibold">Passwort wiederholen</span><input name="confirmation" required type="password" minLength={15} maxLength={72} autoComplete="new-password" className="bank-input" /></label>
			<button disabled={loading || !token} className="bank-primary w-full">{loading ? "Bitte warten…" : "Passwort speichern"}</button>
		</form>}
		{message && <div role="alert" className={`mt-5 rounded-lg border p-3 text-sm ${success ? "border-emerald-200 bg-emerald-50 text-emerald-800" : "border-red-200 bg-red-50 text-red-700"}`}>{message}</div>}
		<Link href="/auth" className="mt-5 block text-center text-sm font-semibold text-[#005b96] hover:underline">Zur Anmeldung</Link>
	</div>;
}

export default function ResetPasswordPage() {
	return <main className="flex min-h-screen items-center justify-center bg-[#eef3f7] p-5">
		<Suspense fallback={<div className="text-sm text-slate-500">Wird geladen…</div>}><ResetPasswordForm /></Suspense>
	</main>;
}
