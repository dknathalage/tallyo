class AnnouncerStore {
	politeMessage = $state('');
	assertiveMessage = $state('');

	announce(message: string, priority: 'polite' | 'assertive' = 'polite') {
		if (priority === 'assertive') {
			this.assertiveMessage = '';
			// Use microtask to ensure DOM update cycle resets the region
			queueMicrotask(() => {
				this.assertiveMessage = message;
			});
		} else {
			this.politeMessage = '';
			queueMicrotask(() => {
				this.politeMessage = message;
			});
		}
	}
}

export const announcer = new AnnouncerStore();
