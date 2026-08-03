/**
 * Core TypeScript Types for Double-Entry Bank
 */

export interface User {
  email: string;
  authenticated: boolean;
	role: "CUSTOMER" | "ADMIN";
}

export interface AdminUser {
	id: string;
	email: string;
	full_name: string;
	role: "CUSTOMER" | "ADMIN";
	created_at: string;
	account_count: number;
	total_balance: string;
}

export interface AdminAccount {
	id: string;
	owner_id: string;
	owner_email: string;
	owner_name: string;
	name: string;
	iban: string;
	account_type: string;
	status: "ACTIVE" | "BLOCKED";
	balance: string;
	available_balance: string;
	created_at: string;
	updated_at: string;
}

export interface AdminOverview {
	users: AdminUser[];
	accounts: AdminAccount[];
	payment_count: number;
}

export interface Account {
  id: string;
  name: string;
  balance: string;
  currency: string;
  created_at: string;
  owner_id: string;
  is_system: boolean;
	available_balance: string;
	iban?: string;
	masked_iban: string;
	account_type: "GIROKONTO" | "SPARKONTO" | "SETTLEMENT";
	status: "ACTIVE" | "BLOCKED" | "CLOSED";
	updated_at: string;
}

export interface Entry {
  id: string;
  account_id: string;
  debit: string;
  credit: string;
  description: string;
  transaction_id: string;
  operation_type: string;
  created_at: string;
	payment_order_id?: string;
	counterparty_name?: string;
	counterparty_iban?: string;
	purpose?: string;
	category?: string;
	booking_date?: string;
	execution_date?: string;
}

export type VoPStatus = "MATCH" | "CLOSE_MATCH" | "NO_MATCH" | "OTHER";

export interface VoPResult {
	result: VoPStatus;
	suggested_name?: string;
	demo_notice: string;
}

export interface Payment {
	id: string;
	source_account_id: string;
	beneficiary_name: string;
	beneficiary_iban?: string;
	masked_beneficiary_iban: string;
	beneficiary_bic?: string;
	amount: string;
	currency: "EUR";
	payment_kind: "UMBUCHUNG" | "INTERNAL" | "SEPA" | "SEPA_INSTANT";
	schedule_type: "IMMEDIATE" | "SCHEDULED" | "STANDING";
	purpose?: string;
	creditor_reference?: string;
	end_to_end_id: string;
	requested_execution_at: string;
	vop_result: VoPStatus;
	vop_suggested_name?: string;
	vop_overridden: boolean;
	status: "DRAFT" | "AWAITING_CONFIRMATION" | "SCHEDULED" | "PROCESSING" | "BOOKED" | "FAILED" | "CANCELLED";
	reject_code?: string;
	failure_reason?: string;
	ledger_transaction_id?: string;
	created_at: string;
	updated_at: string;
	processed_at?: string;
	demo: true;
}

export interface StandingOrder {
	id: string;
	source_account_id: string;
	beneficiary_name: string;
	masked_beneficiary_iban: string;
	amount: string;
	currency: "EUR";
	purpose?: string;
	transfer_type: "STANDARD" | "INSTANT";
	frequency: "WEEKLY" | "MONTHLY" | "QUARTERLY" | "YEARLY";
	start_date: string;
	end_date?: string;
	max_occurrences?: number;
	occurrences_created: number;
	next_execution_at: string;
	status: "ACTIVE" | "PAUSED" | "CANCELLED" | "COMPLETED";
}

export interface Beneficiary {
	id: string;
	name: string;
	iban: string;
	bic?: string;
	category?: string;
	is_demo: boolean;
}

export interface PaymentRequest {
	source_account_id: string;
	beneficiary_name: string;
	beneficiary_iban: string;
	beneficiary_bic?: string;
	amount: string;
	transfer_type: "STANDARD" | "INSTANT";
	schedule_type: "IMMEDIATE" | "SCHEDULED";
	purpose?: string;
	creditor_reference?: string;
	requested_execution_at?: string;
}

export interface StandingOrderRequest {
	source_account_id: string;
	beneficiary_name: string;
	beneficiary_iban: string;
	beneficiary_bic?: string;
	amount: string;
	purpose?: string;
	creditor_reference?: string;
	transfer_type: "STANDARD" | "INSTANT";
	frequency: "WEEKLY" | "MONTHLY" | "QUARTERLY" | "YEARLY";
	start_date: string;
	end_date?: string;
	max_occurrences?: number;
}

export interface Transaction {
  id: string;
  from_account_id: string;
  to_account_id: string;
  amount: number;
  created_at: string;
}

// API Response types
export interface RegisterResponse {
  email: string;
  user_id: string;
}

export interface SessionResponse {
  user_id: string;
	email: string;
	role: "CUSTOMER" | "ADMIN";
}

export interface MessageResponse {
  message: string;
}

export interface ReconcileResponse {
  matched: boolean;
  message: string;
}

export interface ApiErrorResponse {
  error: string;
}

// API Request/Response wrapper
export interface ApiResponse<T> {
  response: Response;
  data: T;
}
