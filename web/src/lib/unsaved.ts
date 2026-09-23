import { beforeNavigate } from '$app/navigation';

/**
 * Asks before leaving an editor with unsaved changes: in the dashboard with a
 * confirm, and when closing or reloading the tab with the browser's own
 * prompt. Navigations the code starts itself (goto after saving) pass.
 * Call it while a component initialises.
 */
export function guardUnsaved(isDirty: () => boolean) {
	beforeNavigate((nav) => {
		if (nav.type === 'goto' || !isDirty()) return;
		if (nav.type === 'leave') {
			nav.cancel();
			return;
		}
		if (!confirm('You have unsaved changes. Leave without saving them?')) nav.cancel();
	});
}
