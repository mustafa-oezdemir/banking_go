"use client";

import { useEffect } from "react";
import { clearPageRecoveryMarker, reloadOnceAfterPageError } from "@/lib/pageRecovery";

export default function ErrorPage({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
	useEffect(() => {
		reloadOnceAfterPageError();
	}, []);

	const retry = () => {
		clearPageRecoveryMarker();
		reset();
	};

	return (
		<main className="flex min-h-screen items-center justify-center bg-[#f3f5f7] px-5 text-[#17212b]">
			<section className="w-full max-w-lg rounded-2xl border border-slate-200 bg-white p-8 text-center shadow-sm">
				<div className="mx-auto flex h-12 w-12 items-center justify-center rounded-xl bg-[#003b70] text-xl font-bold text-white">P</div>
				<h1 className="mt-5 text-2xl font-bold">Die Seite konnte nicht geladen werden</h1>
				<p className="mt-3 text-sm leading-6 text-slate-600">
					Die Verbindung wurde automatisch erneut aufgebaut. Falls die Seite weiterhin nicht erscheint, melden Sie sich bitte erneut an.
				</p>
				<div className="mt-6 flex flex-col justify-center gap-3 sm:flex-row">
					<button type="button" onClick={retry} className="rounded-lg bg-[#0066a1] px-5 py-3 text-sm font-semibold text-white hover:bg-[#005588]">
						Erneut versuchen
					</button>
					<a href="/auth" className="rounded-lg border border-slate-300 px-5 py-3 text-sm font-semibold text-slate-700 hover:bg-slate-50">
						Zur Anmeldung
					</a>
				</div>
			</section>
		</main>
	);
}
