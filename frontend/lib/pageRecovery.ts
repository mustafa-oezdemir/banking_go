export const PAGE_RECOVERY_KEY = "pehlione-page-recovery-at";

const recoveryWindowMs = 30_000;

export function reloadOnceAfterPageError(): void {
	const previousAttempt = Number(sessionStorage.getItem(PAGE_RECOVERY_KEY));
	if (Number.isFinite(previousAttempt) && Date.now() - previousAttempt < recoveryWindowMs) return;

	sessionStorage.setItem(PAGE_RECOVERY_KEY, String(Date.now()));
	window.location.reload();
}

export function clearPageRecoveryMarker(): void {
	sessionStorage.removeItem(PAGE_RECOVERY_KEY);
}
