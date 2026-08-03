"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@/lib/store/authStore";
import {
	cancelPayment,
	deleteStandingOrder,
	getAccountTransactions,
	getAccounts,
	getBeneficiaries,
	getPayments,
	getStandingOrders,
	getSession,
	getAdminOverview,
	updateAdminUserRole,
	updateAdminAccountStatus,
	adjustAdminAccountBalance,
	logoutSession,
	updateStandingOrder,
} from "@/lib/api";
import { formatCurrency, formatDate } from "@/lib/utils";
import type { Account, AdminOverview, Beneficiary, Entry, Payment, StandingOrder } from "@/lib/types";
import { TransferWizard } from "./TransferWizard";

type View = "overview" | "accounts" | "transactions" | "transfer" | "scheduled" | "standing" | "beneficiaries" | "profile" | "admin";

const customerNavigation: Array<{ id: View; label: string; icon: string }> = [
	{ id: "overview", label: "Übersicht", icon: "⌂" },
	{ id: "accounts", label: "Konten", icon: "▣" },
	{ id: "transactions", label: "Umsätze", icon: "↕" },
	{ id: "transfer", label: "Überweisen", icon: "→" },
	{ id: "scheduled", label: "Terminüberweisungen", icon: "◷" },
	{ id: "standing", label: "Daueraufträge", icon: "↻" },
	{ id: "beneficiaries", label: "Empfänger", icon: "♙" },
	{ id: "profile", label: "Profil und Sicherheit", icon: "⚙" },
];

export function BankingApp() {
	const router = useRouter();
	const email = useAuthStore((state) => state.user?.email ?? "");
	const role = useAuthStore((state) => state.user?.role ?? "CUSTOMER");
	const setRole = useAuthStore((state) => state.setRole);
	const storedAccounts = useAuthStore((state) => state.accounts);
	const setAccounts = useAuthStore((state) => state.setAccounts);
	const logout = useAuthStore((state) => state.logout);
	const [view, setView] = useState<View>("overview");
	const [entries, setEntries] = useState<Entry[]>([]);
	const [payments, setPayments] = useState<Payment[]>([]);
	const [standingOrders, setStandingOrders] = useState<StandingOrder[]>([]);
	const [beneficiaries, setBeneficiaries] = useState<Beneficiary[]>([]);
	const [loading, setLoading] = useState(true);
	const [menuOpen, setMenuOpen] = useState(false);
	const [live, setLive] = useState(false);
	const [error, setError] = useState("");
	const navigation = useMemo(
		() => role === "ADMIN" ? [...customerNavigation, { id: "admin" as View, label: "Administration", icon: "⚡" }] : customerNavigation,
		[role],
	);

	const loadAll = useCallback(async () => {
		try {
			const sessionResult = await getSession();
			if (sessionResult.response.ok) setRole(sessionResult.data.role);
			const [accountResult, paymentResult, standingResult, beneficiaryResult] = await Promise.all([
				getAccounts(), getPayments(), getStandingOrders(), getBeneficiaries(),
			]);
			if (!accountResult.response.ok) throw new Error("Konten konnten nicht geladen werden.");
			setAccounts(accountResult.data);
			setPayments(paymentResult.response.ok ? paymentResult.data : []);
			setStandingOrders(standingResult.response.ok ? standingResult.data : []);
			setBeneficiaries(beneficiaryResult.response.ok ? beneficiaryResult.data : []);
			const transactionResults = await Promise.all(accountResult.data.map((account) => getAccountTransactions(account.id)));
			const allEntries = transactionResults.flatMap((result) => result.response.ok ? result.data : []);
			allEntries.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
			setEntries(allEntries);
			setError("");
		} catch (loadError) {
			setError(loadError instanceof Error ? loadError.message : "Daten konnten nicht geladen werden.");
		} finally {
			setLoading(false);
		}
	}, [setAccounts, setRole]);

	useEffect(() => {
		const task = queueMicrotask(() => void loadAll());
		return () => void task;
	}, [loadAll]);

	useEffect(() => {
		let polling: ReturnType<typeof setInterval> | undefined;
		const startPolling = () => {
			if (!polling) polling = setInterval(() => void loadAll(), 15000);
		};
		const source = new EventSource("/events", { withCredentials: true });
		source.onopen = () => {
			setLive(true);
			if (polling) { clearInterval(polling); polling = undefined; }
		};
		source.addEventListener("payment", () => void loadAll());
		source.onerror = () => {
			setLive(false);
			source.close();
			startPolling();
		};
		return () => {
			source.close();
			if (polling) clearInterval(polling);
		};
	}, [loadAll]);

	const signOut = async () => {
		try { await logoutSession(); } finally {
			logout();
			router.replace("/auth");
		}
	};

	const totalBalance = storedAccounts.reduce((sum, account) => sum + Number(account.balance), 0);
	const availableBalance = storedAccounts.reduce((sum, account) => sum + Number(account.available_balance), 0);
	const [monthStart] = useState(() => Date.now() - 30 * 86400000);
	const recentEntries = entries.filter((entry) => new Date(entry.created_at).getTime() >= monthStart);
	const income = recentEntries.reduce((sum, entry) => sum + Number(entry.credit), 0);
	const expenses = recentEntries.reduce((sum, entry) => sum + Number(entry.debit), 0);
	const pending = payments.filter((payment) => ["AWAITING_CONFIRMATION", "SCHEDULED", "PROCESSING"].includes(payment.status));
	const selectedLabel = navigation.find((item) => item.id === view)?.label ?? "Übersicht";

	return (
		<div className="min-h-screen bg-[#f3f5f7] text-[#17212b]">
			<div className="bg-[#fff3cd] border-b border-[#f0d98a] px-4 py-2 text-center text-xs font-semibold text-[#634c00]">
				Demo-Banking – kein echtes Bankkonto · Keine echten Zahlungen oder Bankverbindungen
			</div>
			<header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-slate-200 bg-white px-4 lg:px-8">
				<div className="flex items-center gap-3">
					<button className="rounded-lg p-2 lg:hidden" aria-label="Navigation öffnen" onClick={() => setMenuOpen(!menuOpen)}>☰</button>
					<div className="flex h-9 w-9 items-center justify-center rounded-lg bg-[#003b70] text-lg font-bold text-white">P</div>
					<div><div className="font-bold text-[#003b70]">Pehlione DemoBank</div><div className="text-[11px] text-slate-500">SEPA-Simulation</div></div>
				</div>
				<div className="flex items-center gap-3">
					<span className={`hidden rounded-full px-2 py-1 text-xs sm:inline ${live ? "bg-emerald-50 text-emerald-700" : "bg-slate-100 text-slate-600"}`}>{live ? "● Live" : "Polling"}</span>
					<span className="hidden max-w-52 truncate text-sm text-slate-600 md:inline">{email}</span>
					<button onClick={signOut} className="rounded-lg border border-slate-300 px-3 py-2 text-sm hover:bg-slate-50">Abmelden</button>
				</div>
			</header>
			<div className="mx-auto flex max-w-[1600px]">
				<aside className={`${menuOpen ? "fixed inset-y-0 left-0 z-40 mt-[97px] block shadow-xl" : "hidden"} min-h-[calc(100vh-97px)] w-64 shrink-0 border-r border-slate-200 bg-white p-4 lg:sticky lg:top-16 lg:block lg:h-[calc(100vh-64px)]`}>
					<nav aria-label="Hauptnavigation" className="space-y-1">
						{navigation.map((item) => <button key={item.id} onClick={() => { setView(item.id); setMenuOpen(false); }} className={`flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm transition ${view === item.id ? "bg-[#e8f1f8] font-semibold text-[#003b70]" : "text-slate-600 hover:bg-slate-50"}`}><span className="w-5 text-center text-lg">{item.icon}</span>{item.label}</button>)}
					</nav>
					<div className="mt-8 rounded-xl bg-slate-50 p-3 text-xs text-slate-500"><strong className="block text-slate-700">Nur Demo</strong>IBANs und Umsätze sind vollständig fiktiv.</div>
				</aside>
				<main className="min-w-0 flex-1 p-4 md:p-6 lg:p-8">
					<div className="mb-6 flex flex-wrap items-end justify-between gap-3"><div><p className="text-sm text-slate-500">Online-Banking</p><h1 className="text-2xl font-bold tracking-tight md:text-3xl">{selectedLabel}</h1></div><p className="text-xs text-slate-500">Stand: {new Date().toLocaleString("de-DE")}</p></div>
					{error && <div role="alert" className="mb-5 rounded-xl border border-red-200 bg-red-50 p-3 text-sm text-red-700">{error}</div>}
					{loading ? <Loading /> : (
						<>
							{view === "overview" && <Overview accounts={storedAccounts} entries={entries} payments={payments} total={totalBalance} available={availableBalance} income={income} expenses={expenses} pending={pending.length} onNavigate={setView} />}
							{view === "accounts" && <Accounts accounts={storedAccounts} />}
							{view === "transactions" && <Transactions entries={entries} />}
							{view === "transfer" && <TransferWizard accounts={storedAccounts} onComplete={loadAll} />}
							{view === "scheduled" && <Scheduled payments={payments} onCancel={async (id) => { await cancelPayment(id); await loadAll(); }} onCreate={() => setView("transfer")} />}
							{view === "standing" && <Standing orders={standingOrders} onCreate={() => setView("transfer")} onToggle={async (order) => { await updateStandingOrder(order.id, { amount: order.amount, purpose: order.purpose, status: order.status === "ACTIVE" ? "PAUSED" : "ACTIVE", end_date: order.end_date, max_occurrences: order.max_occurrences }); await loadAll(); }} onDelete={async (id) => { await deleteStandingOrder(id); await loadAll(); }} />}
							{view === "beneficiaries" && <Beneficiaries items={beneficiaries} />}
							{view === "profile" && <Profile email={email} />}
							{view === "admin" && role === "ADMIN" && <AdminPanel />}
						</>
					)}
				</main>
			</div>
		</div>
	);
}

function Card({ children, className = "" }: { children: React.ReactNode; className?: string }) {
	return <section className={`rounded-2xl border border-slate-200 bg-white shadow-[0_1px_3px_rgba(15,23,42,.06)] ${className}`}>{children}</section>;
}

function Overview({ accounts, entries, payments, total, available, income, expenses, pending, onNavigate }: { accounts: Account[]; entries: Entry[]; payments: Payment[]; total: number; available: number; income: number; expenses: number; pending: number; onNavigate: (view: View) => void }) {
	const categories = useMemo(() => {
		const totals = new Map<string, number>();
		for (const entry of entries) if (Number(entry.debit) > 0) totals.set(entry.category || "Sonstiges", (totals.get(entry.category || "Sonstiges") || 0) + Number(entry.debit));
		return [...totals.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5);
	}, [entries]);
	const maxCategory = Math.max(...categories.map(([, amount]) => amount), 1);
	return <div className="space-y-6">
		<div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
			<Stat label="Gesamtsaldo" value={formatCurrency(total)} accent="blue" />
			<Stat label="Verfügbarer Betrag" value={formatCurrency(available)} accent="blue" />
			<Stat label="Einnahmen · 30 Tage" value={formatCurrency(income)} accent="green" />
			<Stat label="Ausgaben · 30 Tage" value={formatCurrency(expenses)} accent="red" />
			<Stat label="Vorgemerkt" value={String(pending)} accent="amber" />
		</div>
		<div className="grid gap-6 xl:grid-cols-[1.35fr_.65fr]">
			<Card className="p-5"><div className="mb-4 flex items-center justify-between"><h2 className="font-bold">Meine Konten</h2><button onClick={() => onNavigate("accounts")} className="text-sm font-semibold text-[#0066a1]">Alle Konten</button></div><div className="grid gap-3 md:grid-cols-2">{accounts.map((account) => <AccountTile key={account.id} account={account} />)}</div></Card>
			<Card className="p-5"><h2 className="mb-4 font-bold">Ausgaben nach Kategorie</h2>{categories.length ? <div className="space-y-4">{categories.map(([category, amount]) => <div key={category}><div className="mb-1 flex justify-between text-sm"><span>{category}</span><span className="font-semibold">{formatCurrency(amount)}</span></div><div className="h-2 rounded-full bg-slate-100"><div className="h-2 rounded-full bg-[#0072a8]" style={{ width: `${Math.max(8, amount / maxCategory * 100)}%` }} /></div></div>)}</div> : <Empty text="Noch keine Ausgaben" />}</Card>
		</div>
		<Card className="overflow-hidden"><div className="flex items-center justify-between border-b border-slate-100 p-5"><h2 className="font-bold">Letzte Umsätze</h2><button onClick={() => onNavigate("transactions")} className="text-sm font-semibold text-[#0066a1]">Alle Umsätze</button></div><TransactionRows entries={entries.slice(0, 8)} /></Card>
		{payments.some((p) => p.status === "FAILED") && <div className="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800">Mindestens eine Demo-Zahlung konnte nicht gebucht werden. Details finden Sie unter Terminüberweisungen.</div>}
	</div>;
}

function Stat({ label, value, accent }: { label: string; value: string; accent: "blue" | "green" | "red" | "amber" }) {
	const colors = { blue: "border-l-[#0066a1]", green: "border-l-emerald-500", red: "border-l-rose-500", amber: "border-l-amber-500" };
	return <Card className={`border-l-4 p-4 ${colors[accent]}`}><p className="text-xs font-medium text-slate-500">{label}</p><p className="mt-2 text-xl font-bold tabular-nums">{value}</p></Card>;
}

function AccountTile({ account }: { account: Account }) {
	return <div className="rounded-xl border border-slate-200 p-4"><div className="flex justify-between"><div><p className="font-semibold">{account.name}</p><p className="mt-1 font-mono text-xs text-slate-500">{account.masked_iban}</p></div><span className="h-fit rounded-full bg-emerald-50 px-2 py-1 text-[10px] font-bold text-emerald-700">{account.status}</span></div><p className="mt-5 text-2xl font-bold tabular-nums">{formatCurrency(account.balance)}</p><p className="mt-1 text-xs text-slate-500">Verfügbar {formatCurrency(account.available_balance)}</p></div>;
}

function Accounts({ accounts }: { accounts: Account[] }) {
	return <div className="grid gap-5 lg:grid-cols-2">{accounts.map((account) => <Card key={account.id} className="overflow-hidden"><div className="bg-gradient-to-r from-[#003b70] to-[#0066a1] p-5 text-white"><div className="flex justify-between"><div><p className="text-sm text-blue-100">{account.account_type === "SPARKONTO" ? "Sparkonto" : "Girokonto"}</p><h2 className="mt-1 text-lg font-bold">{account.name}</h2></div><span className="rounded-full bg-white/15 px-3 py-1 text-xs">Demo</span></div><p className="mt-8 font-mono text-sm tracking-wide">{account.masked_iban}</p></div><div className="grid grid-cols-2 gap-4 p-5"><div><p className="text-xs text-slate-500">Kontostand</p><p className="mt-1 font-bold">{formatCurrency(account.balance)}</p></div><div><p className="text-xs text-slate-500">Verfügbar</p><p className="mt-1 font-bold">{formatCurrency(account.available_balance)}</p></div></div></Card>)}</div>;
}

function Transactions({ entries }: { entries: Entry[] }) {
	const [direction, setDirection] = useState("ALL");
	const [category, setCategory] = useState("ALL");
	const [query, setQuery] = useState("");
	const categories = [...new Set(entries.map((entry) => entry.category).filter(Boolean))];
	const filtered = entries.filter((entry) => (direction === "ALL" || (direction === "INCOMING" ? Number(entry.credit) > 0 : Number(entry.debit) > 0)) && (category === "ALL" || entry.category === category) && `${entry.counterparty_name} ${entry.purpose} ${entry.description}`.toLowerCase().includes(query.toLowerCase()));
	return <Card className="overflow-hidden"><div className="grid gap-3 border-b border-slate-100 p-4 md:grid-cols-3"><input aria-label="Umsätze durchsuchen" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Empfänger oder Verwendungszweck" className="bank-input" /><select aria-label="Richtung" value={direction} onChange={(e) => setDirection(e.target.value)} className="bank-input"><option value="ALL">Alle Richtungen</option><option value="INCOMING">Eingang</option><option value="OUTGOING">Ausgang</option></select><select aria-label="Kategorie" value={category} onChange={(e) => setCategory(e.target.value)} className="bank-input"><option value="ALL">Alle Kategorien</option>{categories.map((item) => <option key={item}>{item}</option>)}</select></div><TransactionRows entries={filtered} /></Card>;
}

function TransactionRows({ entries }: { entries: Entry[] }) {
	if (!entries.length) return <Empty text="Keine Umsätze gefunden" />;
	return <div className="divide-y divide-slate-100">{entries.map((entry) => { const incoming = Number(entry.credit) > 0; const amount = incoming ? entry.credit : entry.debit; return <div key={entry.id} className="grid gap-2 p-4 hover:bg-slate-50 md:grid-cols-[1fr_180px_140px] md:items-center"><div className="min-w-0"><p className="truncate font-semibold">{entry.counterparty_name || entry.description || "Demo-Buchung"}</p><p className="truncate text-sm text-slate-500">{entry.purpose || entry.description} {entry.counterparty_iban && `· ${entry.counterparty_iban}`}</p><div className="mt-1 flex gap-2 text-[11px] text-slate-400"><span>{entry.category || "Sonstiges"}</span><span>·</span><span>{entry.operation_type}</span></div></div><div className="text-xs text-slate-500"><p>Buchung: {formatDate(entry.booking_date || entry.created_at)}</p><p>Ausführung: {formatDate(entry.execution_date || entry.created_at)}</p></div><p className={`text-right font-bold tabular-nums ${incoming ? "text-emerald-600" : "text-rose-600"}`}>{incoming ? "+" : "−"}{formatCurrency(amount)}</p></div>; })}</div>;
}

function Scheduled({ payments, onCancel, onCreate }: { payments: Payment[]; onCancel: (id: string) => Promise<void>; onCreate: () => void }) {
	const items = payments.filter((payment) => payment.schedule_type === "SCHEDULED");
	return <div className="space-y-4"><div className="flex justify-end"><button onClick={onCreate} className="bank-primary">Neue Terminüberweisung</button></div><Card className="overflow-hidden">{items.length ? <div className="divide-y divide-slate-100">{items.map((payment) => <div key={payment.id} className="grid gap-3 p-4 md:grid-cols-[1fr_180px_130px_auto] md:items-center"><div><p className="font-semibold">{payment.beneficiary_name}</p><p className="text-sm text-slate-500">{payment.masked_beneficiary_iban} · {payment.purpose || "Ohne Verwendungszweck"}</p></div><div className="text-sm"><p>{new Date(payment.requested_execution_at).toLocaleString("de-DE")}</p><Status value={payment.status} /></div><p className="font-bold">{formatCurrency(payment.amount)}</p>{["SCHEDULED", "AWAITING_CONFIRMATION"].includes(payment.status) ? <button onClick={() => onCancel(payment.id)} className="text-sm font-semibold text-rose-600">Stornieren</button> : <span className="text-xs text-slate-400">Nicht stornierbar</span>}</div>)}</div> : <Empty text="Keine Terminüberweisungen" />}</Card></div>;
}

function Standing({ orders, onCreate, onToggle, onDelete }: { orders: StandingOrder[]; onCreate: () => void; onToggle: (order: StandingOrder) => Promise<void>; onDelete: (id: string) => Promise<void> }) {
	return <div className="space-y-4"><div className="flex justify-end"><button onClick={onCreate} className="bank-primary">Neuer Dauerauftrag</button></div><div className="grid gap-4 lg:grid-cols-2">{orders.map((order) => <Card key={order.id} className="p-5"><div className="flex justify-between"><div><h2 className="font-bold">{order.beneficiary_name}</h2><p className="mt-1 text-sm text-slate-500">{order.masked_beneficiary_iban}</p></div><Status value={order.status} /></div><div className="mt-5 grid grid-cols-2 gap-3 text-sm"><div><p className="text-xs text-slate-500">Betrag</p><p className="font-bold">{formatCurrency(order.amount)}</p></div><div><p className="text-xs text-slate-500">Turnus</p><p className="font-semibold">{frequencyLabel(order.frequency)}</p></div><div className="col-span-2"><p className="text-xs text-slate-500">Nächste Ausführung</p><p>{new Date(order.next_execution_at).toLocaleString("de-DE")}</p></div></div>{["ACTIVE", "PAUSED"].includes(order.status) && <div className="mt-5 flex gap-3 border-t border-slate-100 pt-4"><button onClick={() => onToggle(order)} className="text-sm font-semibold text-[#0066a1]">{order.status === "ACTIVE" ? "Pausieren" : "Fortsetzen"}</button><button onClick={() => onDelete(order.id)} className="text-sm font-semibold text-rose-600">Beenden</button></div>}</Card>)}{!orders.length && <Card><Empty text="Keine Daueraufträge" /></Card>}</div></div>;
}

function Beneficiaries({ items }: { items: Beneficiary[] }) {
	return <Card className="overflow-hidden">{items.length ? <div className="divide-y divide-slate-100">{items.map((item) => <div key={item.id} className="flex items-center justify-between gap-3 p-4"><div className="flex min-w-0 items-center gap-3"><div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[#e8f1f8] font-bold text-[#003b70]">{item.name.charAt(0)}</div><div className="min-w-0"><p className="truncate font-semibold">{item.name}</p><p className="font-mono text-xs text-slate-500">{item.iban}</p></div></div><span className="rounded-full bg-slate-100 px-2 py-1 text-xs text-slate-600">{item.category || "Sonstiges"}</span></div>)}</div> : <Empty text="Keine gespeicherten Empfänger" />}</Card>;
}

function Profile({ email }: { email: string }) {
	return <div className="grid gap-5 lg:grid-cols-2"><Card className="p-5"><h2 className="font-bold">Profil</h2><dl className="mt-5 space-y-4 text-sm"><div><dt className="text-slate-500">E-Mail</dt><dd className="mt-1 font-semibold">{email}</dd></div><div><dt className="text-slate-500">Umgebung</dt><dd className="mt-1 font-semibold">Fiktive SEPA-Demo</dd></div></dl></Card><Card className="p-5"><h2 className="font-bold">Sicherheit</h2><ul className="mt-5 space-y-3 text-sm text-slate-600"><li>✓ HttpOnly-Sitzungscookie</li><li>✓ SameSite- und CSRF-Schutz</li><li>✓ Serverseitige Kontoinhaberprüfung</li><li>✓ Idempotente Zahlungsaufträge</li></ul><div className="mt-5 rounded-lg bg-amber-50 p-3 text-xs text-amber-800">Die Demo-Bestätigung ist kein echtes TAN- oder SCA-Verfahren.</div></Card></div>;
}

function AdminPanel() {
	const [overview, setOverview] = useState<AdminOverview | null>(null);
	const [amounts, setAmounts] = useState<Record<string, string>>({});
	const [message, setMessage] = useState("");
	const [loading, setLoading] = useState(true);
	const [adjustingAccountID, setAdjustingAccountID] = useState<string | null>(null);

	const load = useCallback(async () => {
		setLoading(true);
		try {
			const result = await getAdminOverview();
			if (!result.response.ok) throw new Error("Administrationsdaten konnten nicht geladen werden.");
			setOverview(result.data);
			setMessage("");
		} catch (loadError) {
			setMessage(loadError instanceof Error ? loadError.message : "Administration nicht verfügbar.");
		} finally {
			setLoading(false);
		}
	}, []);

	useEffect(() => {
		const task = queueMicrotask(() => void load());
		return () => void task;
	}, [load]);

	const updateRole = async (userId: string, role: "CUSTOMER" | "ADMIN") => {
		const result = await updateAdminUserRole(userId, role);
		if (!result.response.ok) { setMessage("Rolle konnte nicht geändert werden."); return; }
		await load();
	};
	const updateStatus = async (accountId: string, status: "ACTIVE" | "BLOCKED") => {
		const result = await updateAdminAccountStatus(accountId, status);
		if (!result.response.ok) { setMessage("Kontostatus konnte nicht geändert werden."); return; }
		await load();
	};
	const adjust = async (accountId: string, operation: "DEPOSIT" | "WITHDRAW") => {
		const amount = normalizeAdminAmount(amounts[accountId] || "");
		if (!isValidAdminAmount(amount)) return;
		setAdjustingAccountID(accountId);
		try {
			const result = await adjustAdminAccountBalance(accountId, operation, amount);
			if (!result.response.ok) { setMessage("Buchung konnte nicht ausgeführt werden."); return; }
			setAmounts((current) => ({ ...current, [accountId]: "" }));
			await load();
		} finally {
			setAdjustingAccountID(null);
		}
	};

	if (loading && !overview) return <Loading />;
	if (!overview) return <div role="alert" className="rounded-xl border border-red-200 bg-red-50 p-4 text-red-700">{message}</div>;
	return <div className="space-y-6">
		{message && <div role="alert" className="rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">{message}</div>}
		<div className="grid gap-4 sm:grid-cols-3"><Stat label="Benutzer" value={String(overview.users.length)} accent="blue" /><Stat label="Konten" value={String(overview.accounts.length)} accent="green" /><Stat label="Zahlungsaufträge" value={String(overview.payment_count)} accent="amber" /></div>
		<Card className="overflow-hidden"><div className="border-b border-slate-100 p-5"><h2 className="font-bold">Benutzerverwaltung</h2></div><div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead className="bg-slate-50 text-xs uppercase text-slate-500"><tr><th className="p-3">Benutzer</th><th className="p-3">Konten</th><th className="p-3">Gesamtsaldo</th><th className="p-3">Rolle</th></tr></thead><tbody className="divide-y divide-slate-100">{overview.users.map((user) => <tr key={user.id}><td className="p-3"><p className="font-semibold">{user.full_name}</p><p className="text-xs text-slate-500">{user.email}</p></td><td className="p-3">{user.account_count}</td><td className="p-3 font-semibold">{formatCurrency(user.total_balance)}</td><td className="p-3"><select aria-label={`Rolle für ${user.email}`} className="bank-input min-w-32" value={user.role} onChange={(event) => void updateRole(user.id, event.target.value as "CUSTOMER" | "ADMIN")}><option value="CUSTOMER">Kunde</option><option value="ADMIN">Admin</option></select></td></tr>)}</tbody></table></div></Card>
		<Card className="overflow-hidden">
			<div className="border-b border-slate-100 p-5"><h2 className="font-bold">Kontenverwaltung</h2><p className="mt-1 text-xs text-slate-500">Gutschriften und Belastungen werden als ausgeglichene Ledger-Buchungen erfasst.</p></div>
			<div className="divide-y divide-slate-100">{overview.accounts.map((account) => {
				const amount = amounts[account.id] || "";
				const disabled = !isValidAdminAmount(amount) || adjustingAccountID === account.id;
				return <div key={account.id} className="grid gap-4 p-4 lg:grid-cols-[1.15fr_.75fr_140px_minmax(330px,1fr)] lg:items-center">
					<div><p className="font-semibold">{account.name}</p><p className="text-xs text-slate-500">{account.owner_name} · {account.owner_email}</p><p className="mt-1 font-mono text-xs text-slate-400">{account.masked_iban}</p></div>
					<div><p className="font-bold">{formatCurrency(account.balance)}</p><p className="text-xs text-slate-500">{account.account_type}</p></div>
					<select aria-label={`Status für ${account.name}`} className="bank-input" value={account.status} onChange={(event) => void updateStatus(account.id, event.target.value as "ACTIVE" | "BLOCKED")}><option value="ACTIVE">Aktiv</option><option value="BLOCKED">Gesperrt</option></select>
					<div className="grid grid-cols-2 gap-2">
						<input aria-label={`Betrag für ${account.name}`} className="bank-input col-span-2 min-w-0" inputMode="decimal" placeholder="Betrag EUR" value={amount} onChange={(event) => setAmounts((current) => ({ ...current, [account.id]: event.target.value }))} />
						<button type="button" disabled={disabled} aria-label={`Gutschrift für ${account.name}`} className="min-h-11 rounded-lg bg-emerald-600 px-3 text-sm font-semibold text-white transition hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-40" onClick={() => void adjust(account.id, "DEPOSIT")}><span aria-hidden="true">+</span> Gutschrift</button>
						<button type="button" disabled={disabled} aria-label={`Belastung für ${account.name}`} className="min-h-11 rounded-lg bg-rose-600 px-3 text-sm font-semibold text-white transition hover:bg-rose-700 disabled:cursor-not-allowed disabled:opacity-40" onClick={() => void adjust(account.id, "WITHDRAW")}><span aria-hidden="true">−</span> Belastung</button>
					</div>
				</div>;
			})}</div>
		</Card>
	</div>;
}

function normalizeAdminAmount(value: string): string {
	return value.trim().replace(",", ".");
}

function isValidAdminAmount(value: string): boolean {
	const normalized = normalizeAdminAmount(value);
	return /^\d+(?:\.\d{1,2})?$/.test(normalized) && Number(normalized) > 0;
}

function Status({ value }: { value: string }) {
	const tone = value === "BOOKED" || value === "ACTIVE" ? "bg-emerald-50 text-emerald-700" : value === "FAILED" || value === "CANCELLED" ? "bg-rose-50 text-rose-700" : "bg-amber-50 text-amber-700";
	return <span className={`inline-block rounded-full px-2 py-1 text-[10px] font-bold ${tone}`}>{value}</span>;
}

function Empty({ text }: { text: string }) { return <div className="p-10 text-center text-sm text-slate-500">{text}</div>; }
function Loading() { return <div className="flex justify-center p-20"><div className="h-8 w-8 animate-spin rounded-full border-4 border-slate-200 border-t-[#0066a1]" aria-label="Daten werden geladen" /></div>; }
function frequencyLabel(value: string) { return ({ WEEKLY: "Wöchentlich", MONTHLY: "Monatlich", QUARTERLY: "Vierteljährlich", YEARLY: "Jährlich" } as Record<string, string>)[value] || value; }
