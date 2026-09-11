export type Toast = { id: number; kind: 'success' | 'error' | 'info'; message: string };

let nextId = 0;

export const toasts = $state<Toast[]>([]);

export function toast(message: string, kind: Toast['kind'] = 'success') {
	const id = ++nextId;
	toasts.push({ id, kind, message });
	setTimeout(() => dismiss(id), kind === 'error' ? 6000 : 3500);
}

export function dismiss(id: number) {
	const i = toasts.findIndex((t) => t.id === id);
	if (i >= 0) toasts.splice(i, 1);
}
