import type { Metadata, Viewport } from "next";
import { headers } from "next/headers";
import "./globals.css";
import { Providers } from "@/components/Providers";
import { Toast } from "@/components/Toast";

export const metadata: Metadata = {
	title: "Pehlione DemoBank | SEPA-Banking Simulation",
	description: "Fiktive SEPA-Demo mit EUR-Konten, Empfängerprüfung, Überweisungen und Double-Entry Ledger.",
	authors: [{ name: "Mustafa Özdemir" }],
	creator: "Mustafa Özdemir",
	publisher: "Mustafa Özdemir",
	icons: {
		icon: "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><rect width='100' height='100' rx='20' fill='%23003b70'/><text x='50' y='70' text-anchor='middle' font-size='62' fill='white' font-family='sans-serif'>P</text></svg>",
	},
};

export const viewport: Viewport = { width: "device-width", initialScale: 1, maximumScale: 5 };

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
	await headers();
	return <html lang="de" className="h-full antialiased" data-scroll-behavior="smooth"><body className="min-h-screen"><Providers>{children}<Toast /></Providers></body></html>;
}
