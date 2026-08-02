/**
 * API Client
 * Handles all HTTP requests to the backend API with authentication
 */

import { API_ENDPOINTS, STORAGE_KEYS } from "@/lib/config";
import type {
  ApiResponse,
  Account,
  Entry,
  MessageResponse,
  ReconcileResponse,
  RegisterResponse,
  SessionResponse,
	Payment,
	PaymentRequest,
	StandingOrder,
	StandingOrderRequest,
	Beneficiary,
	VoPResult,
} from "@/lib/types";

/**
 * Make an authenticated API request
 * Uses relative paths which are rewritten to the backend by Next.js
 */
export async function request<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<ApiResponse<T>> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((options.headers || {}) as Record<string, string>),
  };

  const method = (options.method || "GET").toUpperCase();
  if (!["GET", "HEAD", "OPTIONS"].includes(method)) {
    headers["X-CSRF-Protection"] = "1";
  }

  let response: Response;
  try {
    // Use relative path - Next.js will rewrite to backend via rewrites config
    response = await fetch(endpoint, {
      cache: "no-store",
      credentials: "same-origin",
      ...options,
      headers,
    });
  } catch (error) {
    const reason =
      error && error instanceof Error ? error.message : "request failed";
    throw new Error(`Request to ${endpoint} failed: ${reason}`);
  }

  // Handle an expired or invalid HttpOnly cookie session.
  const isCredentialEndpoint =
    endpoint === API_ENDPOINTS.LOGIN || endpoint === API_ENDPOINTS.REGISTER;
  if (response.status === 401 && !isCredentialEndpoint) {
    if (typeof window !== "undefined") {
      localStorage.removeItem(STORAGE_KEYS.EMAIL);
      // Emit a custom event that hooks can listen to
      window.dispatchEvent(new Event("auth:logout"));
    }
    throw new Error("Session expired - please login again");
  }

  let data: T;
  const contentType = response.headers.get("Content-Type") || "";
  if (contentType.includes("application/json")) {
    data = await response.json();
  } else {
    const text = await response.text();
    try {
      data = JSON.parse(text);
    } catch {
      data = { message: text } as T;
    }
  }

  return { response, data };
}

/**
 * Register a new user
 */
export async function register(
  email: string,
  password: string,
	fullName?: string,
): Promise<ApiResponse<RegisterResponse>> {
  return request<RegisterResponse>(API_ENDPOINTS.REGISTER, {
    method: "POST",
	body: JSON.stringify({ email, password, full_name: fullName }),
  });
}

/**
 * Login a user
 */
export async function login(
  email: string,
  password: string,
): Promise<ApiResponse<MessageResponse>> {
  return request<MessageResponse>(API_ENDPOINTS.LOGIN, {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

/**
 * Get all user accounts
 */
export async function getAccounts(): Promise<ApiResponse<Account[]>> {
  return request<Account[]>(API_ENDPOINTS.ACCOUNTS);
}

/**
 * Get the current HttpOnly-cookie session
 */
export async function getSession(): Promise<ApiResponse<SessionResponse>> {
  return request<SessionResponse>(API_ENDPOINTS.SESSION);
}

/**
 * End the current browser session
 */
export async function logoutSession(): Promise<ApiResponse<MessageResponse>> {
  return request<MessageResponse>(API_ENDPOINTS.LOGOUT, { method: "POST" });
}

/**
 * Get a single user account
 */
export async function getAccount(
  accountId: string,
): Promise<ApiResponse<Account>> {
  return request<Account>(API_ENDPOINTS.ACCOUNT(accountId));
}

/**
 * Create a new account
 */
export async function createAccount(
  name: string,
): Promise<ApiResponse<Account>> {
  return request<Account>(API_ENDPOINTS.ACCOUNTS, {
    method: "POST",
	body: JSON.stringify({ name, currency: "EUR" }),
  });
}

/**
 * Rename an existing account
 */
export async function updateAccount(
  accountId: string,
  name: string,
): Promise<ApiResponse<Account>> {
  return request<Account>(API_ENDPOINTS.ACCOUNT(accountId), {
    method: "PUT",
    body: JSON.stringify({ name }),
  });
}

/**
 * Delete an unused account
 */
export async function deleteAccount(
  accountId: string,
): Promise<ApiResponse<MessageResponse>> {
  return request<MessageResponse>(API_ENDPOINTS.ACCOUNT(accountId), {
    method: "DELETE",
  });
}

/**
 * Deposit funds into an account
 */
export async function deposit(
  accountId: string,
  amount: number,
): Promise<ApiResponse<MessageResponse>> {
  return request<MessageResponse>(API_ENDPOINTS.DEPOSIT(accountId), {
    method: "POST",
	body: JSON.stringify({ amount: amount.toString(), currency: "EUR" }),
  });
}

/**
 * Withdraw funds from an account
 */
export async function withdraw(
  accountId: string,
  amount: number,
): Promise<ApiResponse<MessageResponse>> {
  return request<MessageResponse>(API_ENDPOINTS.WITHDRAW(accountId), {
    method: "POST",
	body: JSON.stringify({ amount: amount.toString(), currency: "EUR" }),
  });
}

/**
 * Transfer funds between accounts
 */
export async function transfer(
  fromAccountId: string,
  toAccountId: string,
  amount: number,
): Promise<ApiResponse<MessageResponse>> {
  return request<MessageResponse>(API_ENDPOINTS.TRANSFERS, {
    method: "POST",
    body: JSON.stringify({
      from_id: fromAccountId,
      to_id: toAccountId,
      amount: amount.toString(),
		currency: "EUR",
    }),
  });
}

/**
 * Get account entries (transaction history)
 */
export async function getEntries(
  accountId: string,
): Promise<ApiResponse<Entry[]>> {
  return request<Entry[]>(API_ENDPOINTS.ENTRIES(accountId));
}

/**
 * Reconcile an account balance against ledger entries
 */
export async function reconcileAccount(
  accountId: string,
): Promise<ApiResponse<ReconcileResponse>> {
  return request<ReconcileResponse>(API_ENDPOINTS.RECONCILE(accountId));
}

/**
 * Get a full transaction view by transaction ID
 */
export async function getTransaction(
  txId: string,
): Promise<ApiResponse<Entry[]>> {
  return request<Entry[]>(API_ENDPOINTS.TRANSACTIONS(txId));
}

export async function getAccountTransactions(accountId: string): Promise<ApiResponse<Entry[]>> {
	return request<Entry[]>(API_ENDPOINTS.ACCOUNT_TRANSACTIONS(accountId));
}

export async function verifyPayee(name: string, iban: string): Promise<ApiResponse<VoPResult>> {
	return request<VoPResult>(API_ENDPOINTS.VERIFY_PAYEE, {
		method: "POST",
		body: JSON.stringify({ name, iban }),
	});
}

export async function createPayment(input: PaymentRequest, idempotencyKey: string): Promise<ApiResponse<Payment>> {
	return request<Payment>(API_ENDPOINTS.PAYMENTS, {
		method: "POST",
		headers: { "Idempotency-Key": idempotencyKey },
		body: JSON.stringify(input),
	});
}

export async function getPayments(): Promise<ApiResponse<Payment[]>> {
	return request<Payment[]>(API_ENDPOINTS.PAYMENTS);
}

export async function confirmPayment(paymentId: string, acceptVoPMismatch: boolean): Promise<ApiResponse<Payment>> {
	return request<Payment>(API_ENDPOINTS.CONFIRM_PAYMENT(paymentId), {
		method: "POST",
		body: JSON.stringify({ accept_vop_mismatch: acceptVoPMismatch, confirm_demo: true }),
	});
}

export async function cancelPayment(paymentId: string): Promise<ApiResponse<Payment>> {
	return request<Payment>(API_ENDPOINTS.CANCEL_PAYMENT(paymentId), { method: "POST" });
}

export async function getStandingOrders(): Promise<ApiResponse<StandingOrder[]>> {
	return request<StandingOrder[]>(API_ENDPOINTS.STANDING_ORDERS);
}

export async function createStandingOrder(input: StandingOrderRequest): Promise<ApiResponse<StandingOrder>> {
	return request<StandingOrder>(API_ENDPOINTS.STANDING_ORDERS, {
		method: "POST",
		body: JSON.stringify(input),
	});
}

export async function updateStandingOrder(orderId: string, input: { amount: string; purpose?: string; status: "ACTIVE" | "PAUSED"; end_date?: string; max_occurrences?: number }): Promise<ApiResponse<StandingOrder>> {
	return request<StandingOrder>(API_ENDPOINTS.STANDING_ORDER(orderId), {
		method: "PATCH",
		body: JSON.stringify(input),
	});
}

export async function deleteStandingOrder(orderId: string): Promise<ApiResponse<StandingOrder>> {
	return request<StandingOrder>(API_ENDPOINTS.STANDING_ORDER(orderId), { method: "DELETE" });
}

export async function getBeneficiaries(): Promise<ApiResponse<Beneficiary[]>> {
	return request<Beneficiary[]>(API_ENDPOINTS.BENEFICIARIES);
}
