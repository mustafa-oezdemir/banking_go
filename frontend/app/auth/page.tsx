"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { forgotPassword, login, register } from "@/lib/api";
import { useAuthStore } from "@/lib/store/authStore";

type AuthMode = "login" | "register" | "forgot";

export default function AuthPage() {
	const router = useRouter();
	const setAuthenticated = useAuthStore((state) => state.setAuthenticated);
	const authenticated = useAuthStore((state) => state.isAuthenticated());
	const hydrated = useAuthStore((state) => state.isHydrated);
	const [mode, setMode] = useState<AuthMode>("login");
	const [loading, setLoading] = useState(false);
	const [message, setMessage] = useState("");
	const [success, setSuccess] = useState(false);

	useEffect(() => {
		if (hydrated && authenticated) router.replace("/dashboard");
	}, [authenticated, hydrated, router]);

	const changeMode = (nextMode: AuthMode) => {
		setMode(nextMode);
		setMessage("");
		setSuccess(false);
	};

	const submit = async (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		setLoading(true);
		setMessage("");
		setSuccess(false);
		const data = new FormData(event.currentTarget);
		const email = String(data.get("email") || "");
		const password = String(data.get("password") || "");
		try {
			if (mode === "forgot") {
				const result = await forgotPassword(email);
				if (!result.response.ok) throw new Error("Die Anfrage konnte nicht verarbeitet werden.");
				setSuccess(true);
				setMessage("Falls die Adresse registriert ist, wurde eine E-Mail mit einem 15 Minuten gültigen Link versendet.");
				return;
			}

			const result = mode === "login"
				? await login(email, password)
				: await register(email, password, String(data.get("fullName") || ""));
			if (!result.response.ok) {
				const apiError = typeof result.data === "object" && result.data !== null && "error" in result.data
					? String((result.data as { error: unknown }).error)
					: "Anmeldung fehlgeschlagen.";
				throw new Error(apiError);
			}
			setAuthenticated(email);
			router.replace("/dashboard");
		} catch (error) {
			setMessage(error instanceof Error ? error.message : "Verbindung fehlgeschlagen.");
		} finally {
			setLoading(false);
		}
	};

	if (!hydrated || authenticated) return <div className="min-h-screen bg-[#eef3f7]" />;

	const heading = mode === "login" ? "Willkommen zurück" : mode === "register" ? "Demo-Zugang erstellen" : "Passwort zurücksetzen";
	const description = mode === "login"
		? "Melden Sie sich im Demo-Online-Banking an."
		: mode === "register"
			? "Ihre E-Mail wird für Sicherheits- und Kontobewegungsbenachrichtigungen verwendet."
			: "Wir senden Ihnen einen einmaligen, 15 Minuten gültigen Link.";

	return <main className="min-h-screen bg-[#eef3f7]">
		<div className="border-b border-[#e6cf7a] bg-[#fff3cd] px-4 py-2 text-center text-xs font-semibold text-[#634c00]">
			Demo-Banking – kein echtes Bankkonto · Keine echten Überweisungen
		</div>
		<div className="grid min-h-[calc(100vh-33px)] lg:grid-cols-[1.1fr_.9fr]">
			<section className="hidden bg-gradient-to-br from-[#002f56] via-[#004b80] to-[#0072a8] p-12 text-white lg:flex lg:flex-col lg:justify-between">
				<div className="flex items-center gap-3">
					<div className="flex h-11 w-11 items-center justify-center rounded-xl bg-white text-xl font-bold text-[#003b70]">P</div>
					<div><h1 className="text-xl font-bold">Pehlione DemoBank</h1><p className="text-xs text-blue-100">SEPA-Banking sicher simuliert</p></div>
				</div>
				<div className="max-w-xl">
					<p className="text-sm font-semibold uppercase tracking-[.2em] text-blue-200">Fintech-Demonstrator</p>
					<h2 className="mt-4 text-4xl font-bold leading-tight">Deutsches Online-Banking, ohne echtes Geld.</h2>
					<p className="mt-5 text-lg leading-8 text-blue-100">IBAN, Empfängerprüfung, Echtzeit- und Terminüberweisungen sowie doppelte Buchführung in einer geschützten Demo-Umgebung.</p>
					<div className="mt-8 grid grid-cols-2 gap-4 text-sm"><div className="rounded-xl bg-white/10 p-4">✓ Double-Entry Ledger</div><div className="rounded-xl bg-white/10 p-4">✓ Verification of Payee</div><div className="rounded-xl bg-white/10 p-4">✓ EUR & Demo-IBAN</div><div className="rounded-xl bg-white/10 p-4">✓ Keine Bankanbindung</div></div>
				</div>
				<p className="text-xs text-blue-200">Keine BaFin-Zulassung · Kein PSD2-Dienst · Nur Simulation</p>
			</section>
			<section className="flex items-center justify-center p-5 md:p-10">
				<div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-7 shadow-xl md:p-9">
					<div className="mb-7 lg:hidden"><div className="flex items-center gap-3"><div className="flex h-10 w-10 items-center justify-center rounded-lg bg-[#003b70] font-bold text-white">P</div><h1 className="font-bold text-[#003b70]">Pehlione DemoBank</h1></div></div>
					<h2 className="text-2xl font-bold">{heading}</h2>
					<p className="mt-2 text-sm text-slate-500">{description}</p>
					{mode !== "forgot" && <div className="mt-6 grid grid-cols-2 rounded-lg bg-slate-100 p-1">
						<button type="button" onClick={() => changeMode("login")} className={`rounded-md py-2 text-sm font-semibold ${mode === "login" ? "bg-white text-[#003b70] shadow-sm" : "text-slate-500"}`}>Anmelden</button>
						<button type="button" onClick={() => changeMode("register")} className={`rounded-md py-2 text-sm font-semibold ${mode === "register" ? "bg-white text-[#003b70] shadow-sm" : "text-slate-500"}`}>Registrieren</button>
					</div>}
					<form onSubmit={submit} className="mt-6 space-y-5">
						{mode === "register" && <label className="block"><span className="mb-2 block text-sm font-semibold">Vollständiger Name</span><input name="fullName" required maxLength={140} autoComplete="name" className="bank-input" placeholder="Anna Beispiel" /></label>}
						<label className="block"><span className="mb-2 block text-sm font-semibold">E-Mail-Adresse</span><input name="email" required type="email" autoComplete="email" className="bank-input" placeholder="name@beispiel.de" /></label>
						{mode !== "forgot" && <label className="block"><span className="mb-2 block text-sm font-semibold">Passwort</span><input name="password" required type="password" minLength={mode === "register" ? 15 : undefined} maxLength={72} autoComplete={mode === "login" ? "current-password" : "new-password"} className="bank-input" /><span className="mt-1 block text-xs text-slate-400">{mode === "register" ? "Mindestens 15 Zeichen." : "Ihre Zugangsdaten."}</span></label>}
						{message && <div role="alert" className={`rounded-lg border p-3 text-sm ${success ? "border-emerald-200 bg-emerald-50 text-emerald-800" : "border-red-200 bg-red-50 text-red-700"}`}>{message}</div>}
						<button disabled={loading} className="bank-primary w-full">{loading ? "Bitte warten…" : mode === "login" ? "Sicher anmelden" : mode === "register" ? "Demo-Zugang erstellen" : "Reset-Link senden"}</button>
					</form>
					{mode === "login" && <button type="button" onClick={() => changeMode("forgot")} className="mt-4 w-full text-center text-sm font-semibold text-[#005b96] hover:underline">Passwort vergessen?</button>}
					{mode === "forgot" && <button type="button" onClick={() => changeMode("login")} className="mt-4 w-full text-center text-sm font-semibold text-[#005b96] hover:underline">Zurück zur Anmeldung</button>}
					<div className="mt-6 rounded-lg bg-amber-50 p-3 text-xs text-amber-800">Dies ist kein echtes Bankkonto. Verwenden Sie kein Passwort, das Sie bei anderen Diensten nutzen.</div>
				</div>
			</section>
		</div>
	</main>;
}
