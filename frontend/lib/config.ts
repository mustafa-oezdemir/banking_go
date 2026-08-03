/**
 * Application Configuration
 * Contains API base URL resolution and constants
 */

// API Base URL resolution
// Uses NEXT_PUBLIC_API_BASE_URL environment variable in production
// Falls back to same-origin or defaults to Render backend
export function getAPIBaseURL(): string {
  // In production/Vercel, use environment variable (client-side)
  if (typeof window !== "undefined" && process.env.NEXT_PUBLIC_API_BASE_URL) {
    return process.env.NEXT_PUBLIC_API_BASE_URL;
  }

  // If running in Node (SSR), use env var if available
  if (process.env.NEXT_PUBLIC_API_BASE_URL) {
    return process.env.NEXT_PUBLIC_API_BASE_URL;
  }

  // In browser, use same-origin by default
  if (typeof window !== "undefined") {
    const host = window.location.hostname;
    const isVercelHost = host.endsWith(".vercel.app");
    const isCustomFrontendDomain =
      host === "pehlione-banking.com" ||
      host === "www.pehlione-banking.com";

    if (isVercelHost || isCustomFrontendDomain) {
      return "https://banking-go.onrender.com";
    }

    return window.location.origin;
  }

  // Server-side fallback
  return "https://banking-go.onrender.com";
}

export const API_BASE_URL = getAPIBaseURL();

// API Endpoints
export const API_ENDPOINTS = {
  REGISTER: "/register",
  LOGIN: "/login",
  FORGOT_PASSWORD: "/forgot-password",
  RESET_PASSWORD: "/reset-password",
  LOGOUT: "/logout",
  SESSION: "/session",
  ACCOUNTS: "/accounts",
  ACCOUNT: (accountId: string) => `/accounts/${accountId}`,
  DEPOSIT: (accountId: string) => `/accounts/${accountId}/deposit`,
  WITHDRAW: (accountId: string) => `/accounts/${accountId}/withdraw`,
  TRANSFERS: "/transfers",
  ENTRIES: (accountId: string) => `/accounts/${accountId}/entries`,
  RECONCILE: (accountId: string) => `/accounts/${accountId}/reconcile`,
  TRANSACTIONS: (txId: string) => `/transactions/${txId}`,
	ACCOUNT_TRANSACTIONS: (accountId: string) => `/accounts/${accountId}/transactions`,
	VERIFY_PAYEE: "/payees/verify",
	PAYMENTS: "/payments",
	PAYMENT: (paymentId: string) => `/payments/${paymentId}`,
	CONFIRM_PAYMENT: (paymentId: string) => `/payments/${paymentId}/confirm`,
	CANCEL_PAYMENT: (paymentId: string) => `/payments/${paymentId}/cancel`,
	STANDING_ORDERS: "/standing-orders",
	STANDING_ORDER: (orderId: string) => `/standing-orders/${orderId}`,
	BENEFICIARIES: "/beneficiaries",
	EVENTS: "/events",
	ADMIN_OVERVIEW: "/admin/overview",
	ADMIN_USER_ROLE: (userId: string) => `/admin/users/${userId}/role`,
	ADMIN_ACCOUNT_STATUS: (accountId: string) => `/admin/accounts/${accountId}/status`,
	ADMIN_ACCOUNT_BALANCE: (accountId: string) => `/admin/accounts/${accountId}/balance`,
  HEALTH: "/health",
} as const;

// Toast notification duration (milliseconds)
export const TOAST_DURATION = 4000;

// Currency settings
export const CURRENCY = {
	CODE: "EUR",
	SYMBOL: "€",
	LOCALE: "de-DE",
} as const;

// Local storage keys
export const STORAGE_KEYS = {
  EMAIL: "email",
} as const;
