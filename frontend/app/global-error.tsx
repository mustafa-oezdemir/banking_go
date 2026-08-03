"use client";

import { useEffect } from "react";
import { clearPageRecoveryMarker, reloadOnceAfterPageError } from "@/lib/pageRecovery";

export default function GlobalError({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
	useEffect(() => {
		reloadOnceAfterPageError();
	}, []);

	const retry = () => {
		clearPageRecoveryMarker();
		reset();
	};

	return (
		<html lang="de">
			<body style={{ margin: 0, background: "#f3f5f7", color: "#17212b", fontFamily: "Arial, sans-serif" }}>
				<main style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", padding: 20, boxSizing: "border-box" }}>
					<section style={{ width: "100%", maxWidth: 520, padding: 32, borderRadius: 16, border: "1px solid #dbe2e8", background: "white", textAlign: "center" }}>
						<div style={{ width: 48, height: 48, margin: "0 auto", borderRadius: 12, display: "flex", alignItems: "center", justifyContent: "center", background: "#003b70", color: "white", fontSize: 20, fontWeight: 700 }}>P</div>
						<h1 style={{ margin: "20px 0 0", fontSize: 24 }}>Die Seite konnte nicht geladen werden</h1>
						<p style={{ margin: "12px 0 0", color: "#52606d", fontSize: 14, lineHeight: 1.6 }}>Die Anwendung hat die Verbindung automatisch erneut aufgebaut. Sie können den Vorgang wiederholen oder sich neu anmelden.</p>
						<div style={{ marginTop: 24, display: "flex", flexWrap: "wrap", justifyContent: "center", gap: 12 }}>
							<button type="button" onClick={retry} style={{ border: 0, borderRadius: 8, padding: "12px 20px", background: "#0066a1", color: "white", fontWeight: 700, cursor: "pointer" }}>Erneut versuchen</button>
							<a href="/auth" style={{ border: "1px solid #cbd5e1", borderRadius: 8, padding: "12px 20px", color: "#334155", fontWeight: 700, textDecoration: "none" }}>Zur Anmeldung</a>
						</div>
					</section>
				</main>
			</body>
		</html>
	);
}
