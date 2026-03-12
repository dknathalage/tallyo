export type ToastType = 'success' | 'error' | 'warning' | 'info';

export interface Toast {
	id: string;
	message: string;
	type: ToastType;
	duration: number;
}

let toasts = $state<Toast[]>([]);

export function getToasts(): Toast[] {
	return toasts;
}

export function addToast(options: { message: string; type?: ToastType; duration?: number }): string {
	const id = crypto.randomUUID();
	const toast: Toast = {
		id,
		message: options.message,
		type: options.type ?? 'info',
		duration: options.duration ?? 4000
	};
	toasts = [...toasts, toast];

	if (toast.duration > 0) {
		setTimeout(() => removeToast(id), toast.duration);
	}

	return id;
}

export function removeToast(id: string): void {
	toasts = toasts.filter((t) => t.id !== id);
}
