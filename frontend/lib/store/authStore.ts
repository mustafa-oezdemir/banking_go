/**
 * Auth Store - Zustand
 * Centralized state management for authentication and user data
 */

"use client";

import { create } from "zustand";
import type { Account, User } from "@/lib/types";
import { STORAGE_KEYS } from "@/lib/config";
import { normalizeAccounts } from "@/lib/utils";
import { getSession } from "@/lib/api";

export interface AuthStore {
  // State
  user: User | null;
  accounts: Account[];
  isHydrated: boolean;

  // Actions
  setAuthenticated: (email: string, role?: "CUSTOMER" | "ADMIN") => void;
  setRole: (role: "CUSTOMER" | "ADMIN") => void;
  setAccounts: (accounts: Account[]) => void;
  hydrate: () => Promise<void>;
  logout: () => void;
  isAuthenticated: () => boolean;
}

export const useAuthStore = create<AuthStore>((set, get) => ({
  user: null,
  accounts: [],
  isHydrated: false,

  setAuthenticated: (email: string, role = "CUSTOMER") => {
    const normalizedEmail = email.trim().toLowerCase();
    set({ user: { email: normalizedEmail, authenticated: true, role } });
    if (typeof window !== "undefined") {
      localStorage.setItem(STORAGE_KEYS.EMAIL, normalizedEmail);
    }
  },

  setRole: (role) => set((state) => ({
    user: state.user ? { ...state.user, role } : state.user,
  })),

  setAccounts: (accounts: Account[]) => {
    set({ accounts: normalizeAccounts(accounts) });
  },

  hydrate: async () => {
    if (typeof window === "undefined") return;

    const email = localStorage.getItem(STORAGE_KEYS.EMAIL);

    if (!email) {
      set({ isHydrated: true });
      return;
    }

    try {
      const { response, data } = await getSession();
      if (response.ok) {
        set({ user: { email: data.email || email, authenticated: true, role: data.role }, isHydrated: true });
        return;
      }
    } catch {
      // Treat unavailable or invalid sessions as signed out.
    }
    localStorage.removeItem(STORAGE_KEYS.EMAIL);
    set({ user: null, accounts: [], isHydrated: true });
  },

  logout: () => {
    if (typeof window !== "undefined") {
      localStorage.removeItem(STORAGE_KEYS.EMAIL);
    }
    set({
      user: null,
      accounts: [],
    });
  },

  isAuthenticated: () => {
    const { user } = get();
    return user?.authenticated === true;
  },
}));
