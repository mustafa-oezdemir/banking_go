/**
 * Dashboard Main Page
 */

"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@/lib/store/authStore";
import { useToastStore } from "@/lib/store/toastStore";
import { getAccounts, getEntries, reconcileAccount } from "@/lib/api";
import { getAPIBaseURL } from "@/lib/config";
import { Header } from "@/components/dashboard/Header";
import { DashboardStats } from "@/components/dashboard/DashboardStats";
import { AccountsList } from "@/components/dashboard/AccountsList";
import { TransactionHistory } from "@/components/dashboard/TransactionHistory";
import { FunctionForms } from "@/components/dashboard/FunctionForms";
import { CreateAccountModal } from "@/components/dashboard/CreateAccountModal";
import { EditAccountModal } from "@/components/dashboard/EditAccountModal";
import { DeleteAccountModal } from "@/components/dashboard/DeleteAccountModal";
import type { Entry, Account } from "@/lib/types";

export default function DashboardPage() {
  const router = useRouter();
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated());
  const isHydrated = useAuthStore((state) => state.isHydrated);
  const setAccounts = useAuthStore((state) => state.setAccounts);
  const showToast = useToastStore((state) => state.showToast);

  const [entries, setEntries] = useState<Entry[]>([]);
  const [isLoadingTransactions, setIsLoadingTransactions] = useState(false);
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [accountToEdit, setAccountToEdit] = useState<Account | null>(null);
  const [accountToDelete, setAccountToDelete] = useState<Account | null>(null);

  // Check authentication
  useEffect(() => {
    if (isHydrated && !isAuthenticated) {
      router.push("/auth");
    }
  }, [isAuthenticated, isHydrated, router]);

  // Load transactions with timeout to prevent hanging
  const loadTransactions = useCallback(async (accs: Account[]) => {
    // A successful empty account response means there is no transaction history to show.
    if (accs.length === 0) {
      setEntries([]);
      return;
    }

    try {
      setIsLoadingTransactions(true);
      // Create promises with timeout wrapper
      const withTimeout = accs.map(
        (acc) =>
          new Promise<{ data?: Entry[] } | null>((resolve) => {
            const timer = setTimeout(() => resolve(null), 8000); // 8s timeout per request
            getEntries(acc.id)
              .then((result) => {
                clearTimeout(timer);
                resolve(result);
              })
              .catch(() => {
                clearTimeout(timer);
                resolve(null);
              });
          }),
      );

      const results = await Promise.all(withTimeout);

      const allEntries: Entry[] = [];
      let successfulResponses = 0;

      results.forEach((result) => {
        if (result?.data && Array.isArray(result.data)) {
          successfulResponses++;
          allEntries.push(...result.data);
        }
      });

      // Only update entries if we got successful responses from all requested accounts
      // This prevents clearing entries when some accounts timeout or fail
      if (successfulResponses === accs.length && successfulResponses > 0) {
        // Sort by date descending
        allEntries.sort(
          (a, b) =>
            new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
        );
        setEntries(allEntries);
      }
    } catch (error) {
      console.error("Error loading transactions:", error);
    } finally {
      setIsLoadingTransactions(false);
    }
  }, []);

  // Load accounts
  const loadAccounts = useCallback(
    async (skipTransactions = false) => {
      try {
        const { response, data } = await getAccounts();
        if (response.ok) {
          setAccounts(data);

          if (!skipTransactions) {
            await loadTransactions(data);
          }
        }
      } catch (error) {
        console.error("Error loading accounts:", error);
      }
    },
    [setAccounts, loadTransactions],
  );

  // Initial load
  useEffect(() => {
    loadAccounts();
  }, [loadAccounts]);

  // Refresh after mutation
  const handleRefreshAfterMutation = useCallback(async () => {
    // Load accounts and transactions together
    const { response, data } = await getAccounts();
    if (response.ok) {
      setAccounts(data);
      // Load transactions with the updated accounts
      await loadTransactions(data);
    }
  }, [setAccounts, loadTransactions]);

  // Reconcile account
  const handleReconcile = useCallback(
    async (accountId: string, accountName: string) => {
      try {
        const { response, data } = await reconcileAccount(accountId);

        if (response.ok) {
          showToast(
            "Reconciliation complete",
            `${accountName} is balanced`,
            "success",
          );
          await loadAccounts();
        } else {
          const errorMessage =
            typeof data === "object" &&
            data !== null &&
            "error" in data &&
            typeof (data as Record<string, unknown>).error === "string"
              ? ((data as Record<string, unknown>).error as string)
              : "Please try again";
          showToast("Reconciliation failed", errorMessage, "error");
        }
      } catch {
        showToast("Network error", "Please try again", "error");
      }
    },
    [showToast, loadAccounts],
  );

  if (!isHydrated || !isAuthenticated) {
    return null;
  }

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-900 via-purple-900 to-slate-900 text-white flex flex-col">
      <Header />

      <div className="container mx-auto px-4 md:px-6 py-6 md:py-8 flex-1 space-y-3 sm:space-y-6 md:space-y-8">
        {/* Dashboard Stats - Top Section */}
        <DashboardStats transactionCount={entries.length} />

        {/* My Accounts Section */}
        <AccountsList
          onReconcile={handleReconcile}
          onOpenCreateModal={() => setIsCreateModalOpen(true)}
          onEditAccount={setAccountToEdit}
          onDeleteAccount={setAccountToDelete}
        />

        {/* Function Forms Section - Deposit/Withdraw/Transfer */}
        <FunctionForms onSuccess={handleRefreshAfterMutation} />

        {/* Transaction History - Full Width at Bottom */}
        <TransactionHistory
          entries={entries}
          isLoading={isLoadingTransactions}
        />
      </div>

      <CreateAccountModal
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
        onSuccess={handleRefreshAfterMutation}
      />

      <EditAccountModal
        account={accountToEdit}
        onClose={() => setAccountToEdit(null)}
        onSuccess={handleRefreshAfterMutation}
      />

      <DeleteAccountModal
        account={accountToDelete}
        onClose={() => setAccountToDelete(null)}
        onSuccess={handleRefreshAfterMutation}
      />

      {/* Footer */}
      <footer className="container mx-auto px-4 md:px-6 py-6 md:py-8 text-center text-xs md:text-sm text-gray-300 border-t border-white/10 mt-auto">
        <div className="space-y-3">
          {/* API Links */}
          <p className="break-words">
            Learn the backend API in Swagger:
            <a
              href={`${getAPIBaseURL()}/swagger/index.html`}
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-300 hover:text-blue-200 underline ml-1 md:ml-2 inline-block"
            >
              Open API Docs
            </a>
            <span className="text-gray-600"> • </span>
            <a
              href={`${getAPIBaseURL()}/health`}
              target="_blank"
              rel="noopener noreferrer"
              className="text-green-300 hover:text-green-200 underline"
            >
              Health Check
            </a>
          </p>

          {/* Project & Profile Links */}
          <p className="flex flex-wrap items-center justify-center gap-2">
            <a
              href="https://github.com/mustafa-oezdemir/banking_go"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center space-x-2 text-gray-300 hover:text-white transition group"
            >
              <i
                className="fab fa-github text-lg group-hover:scale-110 transition"
                aria-hidden="true"
              ></i>
              <span>GitHub Repository</span>
            </a>
            <span className="text-gray-600">•</span>
            <a
              href="https://pehlione.com/"
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-300 hover:text-blue-200 underline inline-block"
            >
              Portfolio
            </a>
            <span className="text-gray-600">•</span>
            <a
              href="https://www.linkedin.com/in/mustafa-oezdemir/"
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-300 hover:text-blue-200 underline inline-block"
            >
              LinkedIn
            </a>
          </p>

          {/* Copyright */}
          <p className="text-gray-500 text-xs">
            © {new Date().getFullYear()} Mustafa Özdemir. All rights reserved.
          </p>
        </div>
      </footer>
    </div>
  );
}
