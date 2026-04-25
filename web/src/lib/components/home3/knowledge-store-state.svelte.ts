import type { KnowledgeStoreRecord } from '$lib/services/kbService';

function createKnowledgeStoreState() {
	let activeStoreId = $state<number | null>(null);
	let activeStore = $state<KnowledgeStoreRecord | null>(null);

	function setActiveStore(store: KnowledgeStoreRecord | null) {
		activeStore = store;
		activeStoreId = store?.id ?? null;
	}

	function setActiveStoreId(id: number | null, stores: KnowledgeStoreRecord[] = []) {
		activeStoreId = id;
		activeStore = id == null ? null : (stores.find((store) => store.id === id) ?? activeStore);
	}

	function syncActiveStore(stores: KnowledgeStoreRecord[]) {
		if (activeStoreId == null) {
			activeStore = null;
			return;
		}
		activeStore = stores.find((store) => store.id === activeStoreId) ?? null;
		if (!activeStore) activeStoreId = null;
	}

	return {
		get activeStoreId() {
			return activeStoreId;
		},
		get activeStore() {
			return activeStore;
		},
		setActiveStore,
		setActiveStoreId,
		syncActiveStore
	};
}

export const knowledgeStoreState = createKnowledgeStoreState();
