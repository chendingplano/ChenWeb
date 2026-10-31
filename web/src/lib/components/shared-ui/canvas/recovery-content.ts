// A recovery save does not clear the ordinary unsaved-changes indicator.
export function needsRecovery(page: unknown, lastSavedContent: string | undefined): boolean {
	return JSON.stringify(page) !== lastSavedContent;
}
