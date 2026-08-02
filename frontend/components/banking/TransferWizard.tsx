"use client";

import { useMemo, useState } from "react";
import { confirmPayment, createPayment, createStandingOrder, verifyPayee } from "@/lib/api";
import { formatCurrency } from "@/lib/utils";
import type { Account, Payment, StandingOrder, VoPResult } from "@/lib/types";

type Timing = "IMMEDIATE" | "SCHEDULED" | "STANDING";

interface FormState {
	sourceAccountId: string;
	beneficiaryName: string;
	beneficiaryIBAN: string;
	beneficiaryBIC: string;
	amount: string;
	purpose: string;
	creditorReference: string;
	transferType: "STANDARD" | "INSTANT";
	timing: Timing;
	executionAt: string;
	frequency: "WEEKLY" | "MONTHLY" | "QUARTERLY" | "YEARLY";
	startDate: string;
	endMode: "UNLIMITED" | "DATE" | "COUNT";
	endDate: string;
	maxOccurrences: string;
}

const initialForm = (accounts: Account[]): FormState => ({
	sourceAccountId: accounts.find((account) => account.account_type === "GIROKONTO" && Number(account.available_balance) > 0)?.id ?? accounts[0]?.id ?? "",
	beneficiaryName: "",
	beneficiaryIBAN: "",
	beneficiaryBIC: "",
	amount: "",
	purpose: "",
	creditorReference: "",
	transferType: "STANDARD",
	timing: "IMMEDIATE",
	executionAt: "",
	frequency: "MONTHLY",
	startDate: new Date(Date.now() + 86400000).toISOString().slice(0, 10),
	endMode: "UNLIMITED",
	endDate: "",
	maxOccurrences: "",
});

const steps = ["Auftraggeberkonto", "Empfänger", "Zahlungsdetails", "Empfängerprüfung", "Prüfen und freigeben", "Ergebnis"];

export function TransferWizard({ accounts, onComplete }: { accounts: Account[]; onComplete: () => Promise<void> }) {
	const [step, setStep] = useState(1);
	const [form, setForm] = useState<FormState>(() => initialForm(accounts));
	const [vop, setVoP] = useState<VoPResult | null>(null);
	const [mismatchAccepted, setMismatchAccepted] = useState(false);
	const [demoConfirmed, setDemoConfirmed] = useState(false);
	const [submitting, setSubmitting] = useState(false);
	const [error, setError] = useState("");
	const [result, setResult] = useState<Payment | StandingOrder | null>(null);
	const [idempotencyKey, setIdempotencyKey] = useState(() => `web-${crypto.randomUUID()}`);
	const source = accounts.find((account) => account.id === form.sourceAccountId);
	const bicRequired = useMemo(() => needsBIC(form.beneficiaryIBAN), [form.beneficiaryIBAN]);

	const setField = <K extends keyof FormState>(field: K, value: FormState[K]) => setForm((current) => ({ ...current, [field]: value }));

	const next = async () => {
		setError("");
		if (step === 1 && !form.sourceAccountId) return setError("Bitte wählen Sie ein Auftraggeberkonto.");
		if (step === 2) {
			if (!form.beneficiaryName.trim() || normalizeIBAN(form.beneficiaryIBAN).length < 15) return setError("Empfängername und eine gültige IBAN sind erforderlich.");
			if (bicRequired && !form.beneficiaryBIC.trim()) return setError("Für dieses Zielland ist ein BIC erforderlich.");
		}
		if (step === 3) {
			if (!/^\d+(?:[.,]\d{1,2})?$/.test(form.amount) || Number(form.amount.replace(",", ".")) <= 0) return setError("Bitte geben Sie einen positiven EUR-Betrag mit höchstens zwei Nachkommastellen ein.");
			if (form.purpose.length > 140) return setError("Der Verwendungszweck darf höchstens 140 Zeichen enthalten.");
			if (form.timing === "SCHEDULED" && (!form.executionAt || new Date(form.executionAt) <= new Date())) return setError("Der Ausführungszeitpunkt muss in der Zukunft liegen.");
			if (form.timing === "STANDING" && !form.startDate) return setError("Ein Startdatum ist erforderlich.");
			setSubmitting(true);
			try {
				const checked = await verifyPayee(form.beneficiaryName, normalizeIBAN(form.beneficiaryIBAN));
				if (!checked.response.ok) throw new Error(readAPIError(checked.data));
				setVoP(checked.data);
				setStep(4);
			} catch (verifyError) {
				setError(verifyError instanceof Error ? verifyError.message : "Empfängerprüfung fehlgeschlagen.");
			} finally { setSubmitting(false); }
			return;
		}
		if (step === 4 && vop?.result !== "MATCH" && !mismatchAccepted) return setError("Bitte bestätigen Sie die Warnung zur Empfängerprüfung ausdrücklich.");
		setStep((current) => Math.min(6, current + 1));
	};

	const submit = async () => {
		if (!demoConfirmed) return setError("Bitte bestätigen Sie, dass dies nur eine Demo-Zahlung ist.");
		if (!source) return setError("Auftraggeberkonto nicht gefunden.");
		setSubmitting(true);
		setError("");
		try {
			const common = {
				source_account_id: source.id,
				beneficiary_name: form.beneficiaryName.trim(),
				beneficiary_iban: normalizeIBAN(form.beneficiaryIBAN),
				beneficiary_bic: bicRequired ? form.beneficiaryBIC.trim() : undefined,
				amount: form.amount.replace(",", "."),
				purpose: form.purpose.trim() || undefined,
				creditor_reference: form.creditorReference.trim() || undefined,
				transfer_type: form.transferType,
			};
			if (form.timing === "STANDING") {
				const created = await createStandingOrder({
					...common,
					frequency: form.frequency,
					start_date: form.startDate,
					end_date: form.endMode === "DATE" ? form.endDate : undefined,
					max_occurrences: form.endMode === "COUNT" ? Number(form.maxOccurrences) : undefined,
				});
				if (!created.response.ok) throw new Error(readAPIError(created.data));
				setResult(created.data);
			} else {
				const created = await createPayment({
					...common,
					schedule_type: form.timing,
					requested_execution_at: form.timing === "SCHEDULED" ? new Date(form.executionAt).toISOString() : undefined,
				}, idempotencyKey);
				if (!created.response.ok) throw new Error(readAPIError(created.data));
				const confirmed = await confirmPayment(created.data.id, vop?.result !== "MATCH");
				if (!confirmed.response.ok && confirmed.response.status !== 422) throw new Error(readAPIError(confirmed.data));
				setResult(confirmed.data);
			}
			setStep(6);
			await onComplete();
		} catch (submitError) {
			setError(submitError instanceof Error ? submitError.message : "Zahlung konnte nicht angelegt werden.");
		} finally { setSubmitting(false); }
	};

	const reset = () => {
		setForm(initialForm(accounts));
		setStep(1); setVoP(null); setResult(null); setMismatchAccepted(false); setDemoConfirmed(false); setError("");
		setIdempotencyKey(`web-${crypto.randomUUID()}`);
	};

	return <div className="mx-auto max-w-4xl">
		<ol className="mb-6 grid grid-cols-3 gap-2 md:grid-cols-6" aria-label="Überweisungsschritte">{steps.map((label, index) => <li key={label} className={`rounded-lg border px-2 py-2 text-center text-[10px] md:text-xs ${step === index + 1 ? "border-[#0066a1] bg-[#e8f1f8] font-bold text-[#003b70]" : step > index + 1 ? "border-emerald-200 bg-emerald-50 text-emerald-700" : "border-slate-200 bg-white text-slate-400"}`}><span className="block text-sm">{step > index + 1 ? "✓" : index + 1}</span>{label}</li>)}</ol>
		<section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm md:p-7">
			{error && <div role="alert" className="mb-5 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div>}
			{step === 1 && <StepOne accounts={accounts} selected={form.sourceAccountId} onChange={(value) => setField("sourceAccountId", value)} />}
			{step === 2 && <StepTwo form={form} bicRequired={bicRequired} setField={setField} />}
			{step === 3 && <StepThree form={form} setField={setField} />}
			{step === 4 && <StepFour vop={vop} accepted={mismatchAccepted} onAccepted={setMismatchAccepted} />}
			{step === 5 && source && <StepFive form={form} source={source} vop={vop} demoConfirmed={demoConfirmed} onDemoConfirmed={setDemoConfirmed} />}
			{step === 6 && <StepSix result={result} onReset={reset} />}
			{step < 6 && <div className="mt-8 flex justify-between border-t border-slate-100 pt-5"><button type="button" disabled={step === 1 || submitting} onClick={() => setStep((current) => current - 1)} className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-semibold disabled:opacity-30">Zurück</button>{step === 5 ? <button type="button" disabled={submitting} onClick={submit} className="bank-primary">{submitting ? "Wird verarbeitet…" : "Demo-Zahlung freigeben"}</button> : <button type="button" disabled={submitting} onClick={next} className="bank-primary">{submitting ? "Wird geprüft…" : "Weiter"}</button>}</div>}
		</section>
	</div>;
}

function StepOne({ accounts, selected, onChange }: { accounts: Account[]; selected: string; onChange: (value: string) => void }) {
	return <div><h2 className="text-xl font-bold">Auftraggeberkonto wählen</h2><p className="mt-1 text-sm text-slate-500">Name und IBAN werden sicher aus Ihrem Konto übernommen.</p><div className="mt-6 grid gap-3">{accounts.map((account) => <label key={account.id} className={`flex cursor-pointer items-center gap-3 rounded-xl border p-4 ${selected === account.id ? "border-[#0066a1] bg-[#f2f8fc]" : "border-slate-200"}`}><input type="radio" name="source" checked={selected === account.id} onChange={() => onChange(account.id)} /><div className="min-w-0 flex-1"><p className="font-semibold">{account.name}</p><p className="truncate font-mono text-xs text-slate-500">{account.masked_iban}</p></div><div className="text-right"><p className="font-bold">{formatCurrency(account.available_balance)}</p><p className="text-xs text-slate-500">verfügbar</p></div></label>)}</div></div>;
}

function StepTwo({ form, bicRequired, setField }: { form: FormState; bicRequired: boolean; setField: <K extends keyof FormState>(field: K, value: FormState[K]) => void }) {
	return <div><h2 className="text-xl font-bold">Empfänger</h2><div className="mt-6 grid gap-5"><Field label="Empfängername"><input autoFocus className="bank-input" value={form.beneficiaryName} onChange={(e) => setField("beneficiaryName", e.target.value)} maxLength={140} autoComplete="name" /></Field><Field label="IBAN"><input className="bank-input font-mono uppercase" value={form.beneficiaryIBAN} onChange={(e) => setField("beneficiaryIBAN", formatIBANInput(e.target.value))} placeholder="DE00 0000 0000 0000 0000 00" autoComplete="off" /></Field>{bicRequired && <Field label="BIC (für dieses Zielland erforderlich)"><input className="bank-input uppercase" value={form.beneficiaryBIC} onChange={(e) => setField("beneficiaryBIC", e.target.value.toUpperCase())} /></Field>}<p className="text-xs text-slate-500">Für Zahlungen innerhalb EU/EWR wird kein BIC abgefragt. Alle Angaben bleiben in der Demo-Umgebung.</p></div></div>;
}

function StepThree({ form, setField }: { form: FormState; setField: <K extends keyof FormState>(field: K, value: FormState[K]) => void }) {
	return <div><h2 className="text-xl font-bold">Zahlungsdetails</h2><div className="mt-6 grid gap-5 md:grid-cols-2"><Field label="Betrag in EUR"><input className="bank-input" inputMode="decimal" value={form.amount} onChange={(e) => setField("amount", e.target.value)} placeholder="0,00" /></Field><Field label="Überweisungsart"><select className="bank-input" value={form.transferType} onChange={(e) => setField("transferType", e.target.value as FormState["transferType"])}><option value="STANDARD">Standardüberweisung</option><option value="INSTANT">Echtzeitüberweisung</option></select></Field><Field label="Verwendungszweck" className="md:col-span-2"><input className="bank-input" value={form.purpose} onChange={(e) => setField("purpose", e.target.value)} maxLength={140} /><span className="mt-1 block text-right text-xs text-slate-400">{form.purpose.length}/140</span></Field><Field label="Gläubigerreferenz (optional)" className="md:col-span-2"><input className="bank-input" value={form.creditorReference} onChange={(e) => setField("creditorReference", e.target.value)} /></Field><FieldGroup label="Ausführung" className="md:col-span-2"><div className="grid gap-2 sm:grid-cols-3">{(["IMMEDIATE", "SCHEDULED", "STANDING"] as Timing[]).map((timing) => <button type="button" key={timing} onClick={() => setField("timing", timing)} className={`rounded-lg border p-3 text-sm font-semibold ${form.timing === timing ? "border-[#0066a1] bg-[#e8f1f8] text-[#003b70]" : "border-slate-200"}`}>{timing === "IMMEDIATE" ? "Sofort" : timing === "SCHEDULED" ? "Termin" : "Dauerauftrag"}</button>)}</div></FieldGroup>{form.timing === "SCHEDULED" && <Field label="Ausführungszeitpunkt" className="md:col-span-2"><input type="datetime-local" className="bank-input" value={form.executionAt} onChange={(e) => setField("executionAt", e.target.value)} /></Field>}{form.timing === "STANDING" && <><Field label="Häufigkeit"><select className="bank-input" value={form.frequency} onChange={(e) => setField("frequency", e.target.value as FormState["frequency"])}><option value="WEEKLY">Wöchentlich</option><option value="MONTHLY">Monatlich</option><option value="QUARTERLY">Vierteljährlich</option><option value="YEARLY">Jährlich</option></select></Field><Field label="Startdatum"><input type="date" className="bank-input" value={form.startDate} onChange={(e) => setField("startDate", e.target.value)} /></Field><Field label="Ende"><select className="bank-input" value={form.endMode} onChange={(e) => setField("endMode", e.target.value as FormState["endMode"])}><option value="UNLIMITED">Unbefristet</option><option value="DATE">Enddatum</option><option value="COUNT">Anzahl Ausführungen</option></select></Field>{form.endMode === "DATE" && <Field label="Enddatum"><input type="date" className="bank-input" value={form.endDate} onChange={(e) => setField("endDate", e.target.value)} /></Field>}{form.endMode === "COUNT" && <Field label="Anzahl"><input type="number" min="1" className="bank-input" value={form.maxOccurrences} onChange={(e) => setField("maxOccurrences", e.target.value)} /></Field>}</>}</div></div>;
}

function StepFour({ vop, accepted, onAccepted }: { vop: VoPResult | null; accepted: boolean; onAccepted: (value: boolean) => void }) {
	if (!vop) return null;
	const content = {
		MATCH: { title: "Name und IBAN stimmen überein", text: "Der Empfänger wurde in der Demo gefunden.", tone: "border-emerald-200 bg-emerald-50 text-emerald-800", icon: "✓" },
		CLOSE_MATCH: { title: "Ähnlicher Name gefunden", text: `Vorschlag: ${vop.suggested_name || "nicht verfügbar"}`, tone: "border-amber-200 bg-amber-50 text-amber-800", icon: "!" },
		NO_MATCH: { title: "Name stimmt nicht überein", text: "Prüfen Sie die Angaben sorgfältig. Der tatsächliche Kontoinhaber wird nicht offengelegt.", tone: "border-red-200 bg-red-50 text-red-800", icon: "!" },
		OTHER: { title: "Prüfung nicht möglich", text: "Die externe IBAN ist in dieser Simulation nicht bekannt.", tone: "border-amber-200 bg-amber-50 text-amber-800", icon: "?" },
	}[vop.result];
	return <div><h2 className="text-xl font-bold">Empfängerprüfung</h2><div className={`mt-6 rounded-xl border p-5 ${content.tone}`}><div className="flex gap-3"><span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-white/70 font-bold">{content.icon}</span><div><p className="font-bold">{content.title}</p><p className="mt-1 text-sm">{content.text}</p><p className="mt-3 text-xs opacity-75">{vop.demo_notice}</p></div></div></div>{vop.result !== "MATCH" && <label className="mt-5 flex cursor-pointer gap-3 rounded-lg border border-slate-200 p-4 text-sm"><input type="checkbox" checked={accepted} onChange={(e) => onAccepted(e.target.checked)} /><span>Ich habe die Warnung verstanden, die Angaben selbst geprüft und möchte in der Demo fortfahren.</span></label>}</div>;
}

function StepFive({ form, source, vop, demoConfirmed, onDemoConfirmed }: { form: FormState; source: Account; vop: VoPResult | null; demoConfirmed: boolean; onDemoConfirmed: (value: boolean) => void }) {
	return <div><h2 className="text-xl font-bold">Auftrag prüfen</h2><div className="mt-6 divide-y divide-slate-100 rounded-xl border border-slate-200">{[["Von", `${source.name} · ${source.masked_iban}`], ["An", `${form.beneficiaryName} · ${formatIBANInput(form.beneficiaryIBAN)}`], ["Betrag", formatCurrency(form.amount.replace(",", "."))], ["Verwendungszweck", form.purpose || "—"], ["Ausführung", timingLabel(form)], ["Empfängerprüfung", vop?.result || "—"]].map(([label, value]) => <div key={label} className="grid gap-1 p-4 sm:grid-cols-[180px_1fr]"><span className="text-sm text-slate-500">{label}</span><span className="font-semibold">{value}</span></div>)}</div><label className="mt-5 flex cursor-pointer gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900"><input type="checkbox" checked={demoConfirmed} onChange={(e) => onDemoConfirmed(e.target.checked)} /><span><strong>Demo-Auftrag bestätigen.</strong> Ich verstehe, dass kein echtes Geld übertragen wird und dies kein TAN-/SCA-Verfahren ist.</span></label></div>;
}

function StepSix({ result, onReset }: { result: Payment | StandingOrder | null; onReset: () => void }) {
	if (!result) return null;
	const payment = "end_to_end_id" in result ? result : null;
	const success = payment ? ["BOOKED", "SCHEDULED"].includes(payment.status) : result.status === "ACTIVE";
	return <div className="py-6 text-center"><div className={`mx-auto flex h-16 w-16 items-center justify-center rounded-full text-3xl ${success ? "bg-emerald-100 text-emerald-700" : "bg-red-100 text-red-700"}`}>{success ? "✓" : "!"}</div><h2 className="mt-5 text-2xl font-bold">{payment ? payment.status === "BOOKED" ? "Demo-Zahlung gebucht" : payment.status === "SCHEDULED" ? "Termin gespeichert" : "Verarbeitung nicht erfolgreich" : "Dauerauftrag angelegt"}</h2><p className="mt-2 text-sm text-slate-500">Referenz: <span className="font-mono">{payment?.end_to_end_id || result.id}</span></p>{payment?.failure_reason && <p className="mx-auto mt-4 max-w-lg rounded-lg bg-red-50 p-3 text-sm text-red-700">{payment.failure_reason} {payment.reject_code && `(${payment.reject_code})`}</p>}<button onClick={onReset} className="bank-primary mt-7">Neue Überweisung</button></div>;
}

function Field({ label, children, className = "" }: { label: string; children: React.ReactNode; className?: string }) { return <label className={`block ${className}`}><span className="mb-2 block text-sm font-semibold text-slate-700">{label}</span>{children}</label>; }
function FieldGroup({ label, children, className = "" }: { label: string; children: React.ReactNode; className?: string }) { return <fieldset className={className}><legend className="mb-2 text-sm font-semibold text-slate-700">{label}</legend>{children}</fieldset>; }
function normalizeIBAN(value: string) { return value.replace(/\s/g, "").toUpperCase(); }
function formatIBANInput(value: string) { return normalizeIBAN(value).replace(/(.{4})/g, "$1 ").trim(); }
function needsBIC(iban: string) { const country = normalizeIBAN(iban).slice(0, 2); const ewr = new Set(["AT","BE","BG","HR","CY","CZ","DK","EE","FI","FR","DE","GR","HU","IS","IE","IT","LV","LI","LT","LU","MT","NL","NO","PL","PT","RO","SK","SI","ES","SE"]); return country.length === 2 && !ewr.has(country); }
function timingLabel(form: FormState) { if (form.timing === "IMMEDIATE") return form.transferType === "INSTANT" ? "Sofort · Echtzeit" : "Sofort · Standard"; if (form.timing === "SCHEDULED") return new Date(form.executionAt).toLocaleString("de-DE"); return `${form.frequency} ab ${new Date(form.startDate).toLocaleDateString("de-DE")}`; }
function readAPIError(data: unknown) { return typeof data === "object" && data !== null && "error" in data && typeof (data as { error?: unknown }).error === "string" ? (data as { error: string }).error : "Anfrage fehlgeschlagen."; }
