/**
 * Delete Account Modal Component
 */

"use client";

import { useState } from "react";
import { deleteAccount } from "@/lib/api";
import { useToastStore } from "@/lib/store/toastStore";
import type { Account } from "@/lib/types";

interface DeleteAccountModalProps {
  account: Account | null;
  onClose: () => void;
  onSuccess: () => Promise<void>;
}

export function DeleteAccountModal({
  account,
  onClose,
  onSuccess,
}: DeleteAccountModalProps) {
  const [isLoading, setIsLoading] = useState(false);
  const showToast = useToastStore((state) => state.showToast);

  const handleDelete = async () => {
    if (!account) return;

    setIsLoading(true);
    try {
      const { response, data } = await deleteAccount(account.id);
      if (response.ok) {
        showToast(
          "Account deleted",
          `${account.name} was permanently removed`,
          "success",
        );
        onClose();
        await onSuccess();
      } else {
        const errorMessage =
          typeof data === "object" &&
          data !== null &&
          "error" in data &&
          typeof (data as Record<string, unknown>).error === "string"
            ? ((data as Record<string, unknown>).error as string)
            : "Please try again";
        showToast("Deletion failed", errorMessage, "error");
      }
    } catch {
      showToast("Network error", "Please try again", "error");
    } finally {
      setIsLoading(false);
    }
  };

  if (!account) return null;

  return (
    <div
      className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50"
      onClick={isLoading ? undefined : onClose}
      role="presentation"
    >
      <div
        className="bg-slate-900 rounded-2xl p-8 max-w-md w-full mx-4 border border-red-400/30"
        onClick={(event) => event.stopPropagation()}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="delete-account-title"
        aria-describedby="delete-account-description"
      >
        <h3 id="delete-account-title" className="text-2xl font-bold mb-4">
          Delete Account
        </h3>
        <p id="delete-account-description" className="text-gray-300 mb-3">
          Delete <strong className="text-white">{account.name}</strong>?
        </p>
        <p className="text-sm text-amber-200 bg-amber-500/10 border border-amber-400/20 rounded-lg p-3 mb-6">
          Only zero-balance accounts without transaction history can be deleted.
          Ledger records are never removed.
        </p>
        <div className="flex space-x-3">
          <button
            type="button"
            onClick={onClose}
            disabled={isLoading}
            className="flex-1 bg-gray-600 hover:bg-gray-700 py-3 rounded-lg font-semibold transition disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleDelete}
            disabled={isLoading}
            className="flex-1 bg-red-500 hover:bg-red-600 py-3 rounded-lg font-semibold transition disabled:opacity-50"
          >
            {isLoading ? "Deleting..." : "Delete"}
          </button>
        </div>
      </div>
    </div>
  );
}
