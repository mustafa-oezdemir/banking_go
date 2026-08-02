/**
 * Utility Functions
 * Reusable helper functions
 */

import { CURRENCY } from "@/lib/config";
import type { Account } from "@/lib/types";

type AccountInput = Omit<Account, "balance"> & {
  balance: string | number | null | undefined;
};

/**
 * Format a number as currency
 */
export function formatCurrency(amount: number | string): string {
  const num = parseFloat(String(amount)) || 0;
  return (
	num.toLocaleString(CURRENCY.LOCALE, {
		minimumFractionDigits: 2,
		maximumFractionDigits: 2,
		style: "currency",
		currency: CURRENCY.CODE,
	})
  );
}

/**
 * Truncate a string to a specified length
 */
export function truncate(str: string | undefined, length: number = 8): string {
  if (!str || str.length <= length) return str || "";
  return str.substring(0, length) + "...";
}

/**
 * Format a date
 */
export function formatDate(dateString: string): string {
  try {
    return new Date(dateString).toLocaleString();
  } catch {
    return dateString;
  }
}

/**
 * Validate email format
 */
export function isValidEmail(email: string): boolean {
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  return emailRegex.test(email);
}

/**
 * Validate amount
 */
export function isValidAmount(amount: string | number): boolean {
  const num = parseFloat(String(amount));
  return !isNaN(num) && num > 0;
}

/**
 * Normalize account data so balances always use the API's string format
 */
export function normalizeAccounts(accounts: AccountInput[]): Account[] {
  return (accounts || []).map((acc) => ({
    ...acc,
		balance: String(acc.balance ?? 0),
		available_balance: String(acc.available_balance ?? acc.balance ?? 0),
  }));
}
