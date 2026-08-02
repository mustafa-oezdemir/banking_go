/**
 * Edit Account Modal Component
 */

"use client";

import { useEffect, useState } from "react";
import { updateAccount } from "@/lib/api";
import { useToastStore } from "@/lib/store/toastStore";
import type { Account } from "@/lib/types";

interface EditAccountModalProps {
  account: Account | null;
  onClose: () => void;
  onSuccess: () => Promise<void>;
}

export function EditAccountModal({
  account,
  onClose,
  onSuccess,
}: EditAccountModalProps) {
  const [accountName, setAccountName] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const showToast = useToastStore((state) => state.showToast);

  useEffect(() => {
    setAccountName(account?.name ?? "");
  }, [account]);

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    const normalizedName = accountName.trim();
    if (!account || !normalizedName || normalizedName === account.name) return;

    setIsLoading(true);
    try {
      const { response, data } = await updateAccount(
        account.id,
        normalizedName,
      );
      if (response.ok) {
        showToast(
          "Account updated",
          `${account.name} was renamed to ${normalizedName}`,
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
        showToast("Update failed", errorMessage, "error");
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
        className="bg-slate-900 rounded-2xl p-8 max-w-md w-full mx-4 border border-white/20"
        onClick={(event) => event.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="edit-account-title"
      >
        <h3 id="edit-account-title" className="text-2xl font-bold mb-6">
          Edit Account
        </h3>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label
              htmlFor="edit-account-name"
              className="block text-sm font-medium mb-2"
            >
              Account Name
            </label>
            <input
              id="edit-account-name"
              type="text"
              value={accountName}
              onChange={(event) => setAccountName(event.target.value)}
              required
              maxLength={100}
              autoFocus
              disabled={isLoading}
              className="w-full px-4 py-3 rounded-lg bg-white/5 border border-white/10 focus:border-purple-500 focus:outline-none text-white placeholder-gray-500 disabled:opacity-50"
            />
          </div>
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
              type="submit"
              disabled={
                isLoading ||
                !accountName.trim() ||
                accountName.trim() === account.name
              }
              className="flex-1 bg-amber-500 hover:bg-amber-600 py-3 rounded-lg font-semibold transition disabled:opacity-50"
            >
              {isLoading ? "Saving..." : "Save"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
